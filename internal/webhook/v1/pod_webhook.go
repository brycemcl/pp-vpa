/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/eviction"
)

var podlog = logf.Log.WithName("pod-resource")

// SetupPodWebhookWithManager registers the mutating webhook on the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1.Pod{}).
		WithDefaulter(&PodCustomDefaulter{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-k8s-io-v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups=k8s.io,resources=pods,verbs=create;update,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1

// PodCustomDefaulter injects resource requests/limits based on the parent
// PP-VPA's defaultRecommendation, following the state-inheritance rule.
type PodCustomDefaulter struct {
	Client client.Client
}

// Default implements webhook.CustomDefaulter.
func (d *PodCustomDefaulter) Default(ctx context.Context, obj *corev1.Pod) error {
	if obj == nil {
		return nil
	}
	if obj.Annotations[eviction.AnnotationSurgePod] == "true" {
		// Surge pod was already mutated with upperBound by the eviction layer.
		podlog.Info("skipping injection: surge pod", "pod", obj.Name)
		return nil
	}
	ppvpa, err := d.findPPVPA(ctx, obj)
	if err != nil {
		podlog.Error(err, "locating PP-VPA")
		return nil
	}
	if ppvpa == nil || ppvpa.Status.DefaultRecommendation == nil {
		return nil
	}
	useUpper := ppvpa.Status.TemporaryReplicas > 0 || d.siblingsFlagged(ctx, ppvpa)
	rec := ppvpa.Status.DefaultRecommendation

	chosen := rec.ContainerRecommendations
	for i := range obj.Spec.Containers {
		match := matchContainer(chosen, obj.Spec.Containers[i].Name)
		if match == nil {
			continue
		}
		if useUpper {
			obj.Spec.Containers[i].Resources.Requests = match.UpperBound
			obj.Spec.Containers[i].Resources.Limits = match.UpperBound
		} else {
			obj.Spec.Containers[i].Resources.Requests = match.Target
			obj.Spec.Containers[i].Resources.Limits = match.Target
		}
	}
	return nil
}

func (d *PodCustomDefaulter) findPPVPA(ctx context.Context, pod *corev1.Pod) (*autoscalingv1alpha1.PerPodVerticalPodAutoscaler, error) {
	// Walk OwnerReferences: Pod → ReplicaSet → Deployment.
	for _, o := range pod.OwnerReferences {
		if o.Kind != "ReplicaSet" {
			continue
		}
		var rs appsv1.ReplicaSet
		if err := d.Client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: o.Name}, &rs); err != nil {
			continue
		}
		for _, ro := range rs.OwnerReferences {
			if ro.Kind != "Deployment" {
				continue
			}
			return d.findPPVPAByDeployment(ctx, pod.Namespace, ro.Name)
		}
	}
	return nil, nil
}

func (d *PodCustomDefaulter) findPPVPAByDeployment(ctx context.Context, ns, depName string) (*autoscalingv1alpha1.PerPodVerticalPodAutoscaler, error) {
	var list autoscalingv1alpha1.PerPodVerticalPodAutoscalerList
	if err := d.Client.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		t := list.Items[i].Spec.TargetRef
		if t.Kind == "Deployment" && t.Name == depName {
			return &list.Items[i], nil
		}
	}
	return nil, nil
}

// siblingsFlagged reports whether any sibling PRR is Infeasible or Anomalous.
func (d *PodCustomDefaulter) siblingsFlagged(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) bool {
	var prrs autoscalingv1alpha1.PodResourceRecommendationList
	if err := d.Client.List(ctx, &prrs, client.InNamespace(ppvpa.Namespace)); err != nil {
		return false
	}
	for _, p := range prrs.Items {
		owned := false
		for _, o := range p.OwnerReferences {
			if o.UID == ppvpa.UID {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		if conditionTrue(p.Status.Conditions, autoscalingv1alpha1.PRRConditionInfeasible) ||
			conditionTrue(p.Status.Conditions, autoscalingv1alpha1.PRRConditionAnomalous) {
			return true
		}
	}
	return false
}

func conditionTrue(conds []metav1.Condition, t string) bool {
	for _, c := range conds {
		if c.Type == t && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func matchContainer(recs []autoscalingv1alpha1.ContainerRecommendation, name string) *autoscalingv1alpha1.ContainerRecommendation {
	for i := range recs {
		if recs[i].ContainerName == name {
			return &recs[i]
		}
	}
	return nil
}
