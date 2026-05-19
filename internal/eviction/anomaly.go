/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package eviction

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

// AnomalyHandler evicts pods whose Conditions[Anomalous]=True. Uses the
// Eviction API so PDBs still apply.
type AnomalyHandler struct {
	Client client.Client
}

func (h *AnomalyHandler) Handle(ctx context.Context, _ *autoscalingv1alpha1.PerPodVerticalPodAutoscaler, prr *autoscalingv1alpha1.PodResourceRecommendation) error {
	pod := &corev1.Pod{}
	if err := h.Client.Get(ctx, types.NamespacedName{Namespace: prr.Namespace, Name: prr.Spec.TargetPodName}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return evictPod(ctx, h.Client, pod)
}
