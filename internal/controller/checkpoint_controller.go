/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/recommender"
)

// CheckpointReconciler periodically encodes the workload-aggregate histogram
// to a PerPodVerticalPodAutoscalerCheckpoint CR, ensuring restart resilience.
type CheckpointReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalercheckpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalercheckpoints/status,verbs=get;update;patch

func (r *CheckpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var ppvpa autoscalingv1alpha1.PerPodVerticalPodAutoscaler
	if err := r.Get(ctx, req.NamespacedName, &ppvpa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !ppvpa.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	var prrs autoscalingv1alpha1.PodResourceRecommendationList
	if err := r.List(ctx, &prrs, client.InNamespace(ppvpa.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	owned := prrs.Items[:0]
	for _, p := range prrs.Items {
		for _, o := range p.OwnerReferences {
			if o.UID == ppvpa.UID {
				owned = append(owned, p)
				break
			}
		}
	}
	wh, err := recommender.Aggregate(owned)
	if err != nil {
		return ctrl.Result{}, err
	}
	enc, err := recommender.EncodeWorkload(wh, time.Now())
	if err != nil {
		return ctrl.Result{}, err
	}

	cpName := fmt.Sprintf("%s-checkpoint", ppvpa.Name)
	now := metav1.Now()
	cp := &autoscalingv1alpha1.PerPodVerticalPodAutoscalerCheckpoint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ppvpa.Namespace,
			Name:      cpName,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: autoscalingv1alpha1.GroupVersion.String(),
				Kind:       "PerPodVerticalPodAutoscaler",
				Name:       ppvpa.Name,
				UID:        ppvpa.UID,
				Controller: ptrBool(true),
			}},
		},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerCheckpointSpec{
			PPVPARef: ppvpa.Name,
		},
	}
	if err := r.Create(ctx, cp); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, err
	}
	// Read-back, write status.
	if err := r.Get(ctx, types.NamespacedName{Namespace: cp.Namespace, Name: cp.Name}, cp); err != nil {
		return ctrl.Result{}, err
	}
	patch := cp.DeepCopy()
	patch.Status.AggregateHistogramCheckpoint = enc
	patch.Status.LastUpdateTime = &now
	if err := r.Status().Patch(ctx, patch, client.MergeFrom(cp)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *CheckpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.PerPodVerticalPodAutoscaler{}).
		Named("ppvpa-checkpoint").
		Complete(r)
}
