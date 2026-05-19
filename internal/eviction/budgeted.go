/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package eviction implements the budgeted maxSurge "scale-up, then evict"
// fallback used when in-place resize is infeasible, plus the PDB-honoring
// unavailable path and the anomaly watcher.
package eviction

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

// AnnotationSurgePod marks a surge pod so the mutating webhook injects upperBound.
const AnnotationSurgePod = "pp-vpa.brycemclachlan.me/surge"

// BudgetedHandler implements maxSurge: increment temporaryReplicas, wait for the
// surge pod to be Ready, evict the infeasible pod, then decrement.
type BudgetedHandler struct {
	Client client.Client
	// Now is overridable for tests.
	Now func() time.Time
}

// Handle drives one step of the budgeted state machine for the given PRR.
// It is safe to call repeatedly; each call advances state at most once.
func (h *BudgetedHandler) Handle(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler, prr *autoscalingv1alpha1.PodResourceRecommendation) error {
	if h.Now == nil {
		h.Now = time.Now
	}
	maxSurge := resolveBudget(ppvpa.Spec.UpdatePolicy.InfeasibleUpdateBehavior.MaxSurge, ppvpa.Status.ActiveReplicas, defaultPercent(25))
	if ppvpa.Status.TemporaryReplicas < maxSurge {
		patch := ppvpa.DeepCopy()
		patch.Status.TemporaryReplicas = ppvpa.Status.TemporaryReplicas + 1
		if err := h.Client.Status().Patch(ctx, patch, client.MergeFrom(ppvpa)); err != nil {
			return fmt.Errorf("bumping temporaryReplicas: %w", err)
		}
		return nil
	}
	// Locate a surge pod that is Ready, then evict the infeasible target.
	surge, err := h.findSurgePod(ctx, ppvpa)
	if err != nil {
		return err
	}
	if surge == nil || !podReady(surge) {
		// Wait until ready or burstTimeoutSeconds expires.
		if ppvpa.Spec.UpdatePolicy.BurstTimeoutSeconds > 0 && prr.CreationTimestamp.Add(time.Duration(ppvpa.Spec.UpdatePolicy.BurstTimeoutSeconds)*time.Second).Before(h.Now()) {
			// Roll back temporaryReplicas.
			patch := ppvpa.DeepCopy()
			if patch.Status.TemporaryReplicas > 0 {
				patch.Status.TemporaryReplicas--
			}
			return h.Client.Status().Patch(ctx, patch, client.MergeFrom(ppvpa))
		}
		return nil
	}
	// Evict via the unavailable path so PDBs are honored.
	pod := &corev1.Pod{}
	if err := h.Client.Get(ctx, types.NamespacedName{Namespace: prr.Namespace, Name: prr.Spec.TargetPodName}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := evictPod(ctx, h.Client, pod); err != nil {
		return err
	}
	// Decrement temporaryReplicas now that the surge has replaced the infeasible pod.
	patch := ppvpa.DeepCopy()
	if patch.Status.TemporaryReplicas > 0 {
		patch.Status.TemporaryReplicas--
	}
	return h.Client.Status().Patch(ctx, patch, client.MergeFrom(ppvpa))
}

func (h *BudgetedHandler) findSurgePod(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) (*corev1.Pod, error) {
	var pods corev1.PodList
	if err := h.Client.List(ctx, &pods, client.InNamespace(ppvpa.Namespace)); err != nil {
		return nil, err
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Annotations[AnnotationSurgePod] == "true" {
			return p, nil
		}
	}
	return nil, nil
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// resolveBudget converts an IntOrString budget against a base replica count.
func resolveBudget(b *intstr.IntOrString, base int32, dflt intstr.IntOrString) int32 {
	if b == nil {
		x, _ := intstr.GetScaledValueFromIntOrPercent(&dflt, int(base), true)
		return int32(x)
	}
	x, err := intstr.GetScaledValueFromIntOrPercent(b, int(base), true)
	if err != nil {
		return 0
	}
	return int32(x)
}

func defaultPercent(p int) intstr.IntOrString { return intstr.FromString(fmt.Sprintf("%d%%", p)) }
