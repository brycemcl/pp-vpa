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
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/recommender"
)

const ppvpaFinalizer = "autoscaling.brycemclachlan.me/ppvpa-finalizer"

// PerPodVerticalPodAutoscalerReconciler owns the workload-level loop: maintains
// the 1:1 PRR-per-pod invariant, runs the aggregate recommender, and reconciles
// Deployment replicas to targetReplicas + temporaryReplicas (HPA isolation).
type PerPodVerticalPodAutoscalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalers/finalizers,verbs=update
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=podresourcerecommendations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=podresourcerecommendations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalercheckpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=perpodverticalpodautoscalercheckpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/scale,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch

func (r *PerPodVerticalPodAutoscalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var ppvpa autoscalingv1alpha1.PerPodVerticalPodAutoscaler
	if err := r.Get(ctx, req.NamespacedName, &ppvpa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ppvpa.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, &ppvpa)
	}
	if !containsString(ppvpa.Finalizers, ppvpaFinalizer) {
		ppvpa.Finalizers = append(ppvpa.Finalizers, ppvpaFinalizer)
		if err := r.Update(ctx, &ppvpa); err != nil {
			return ctrl.Result{}, err
		}
	}

	pods, err := r.targetedPods(ctx, &ppvpa)
	if err != nil {
		log.Error(err, "listing target pods")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := r.reconcilePRRs(ctx, &ppvpa, pods); err != nil {
		log.Error(err, "reconciling PRRs")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := r.reconcileRecommendation(ctx, &ppvpa); err != nil {
		log.Error(err, "computing recommendation")
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	if err := r.reconcileReplicas(ctx, &ppvpa); err != nil {
		log.Error(err, "reconciling deployment replicas")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *PerPodVerticalPodAutoscalerReconciler) finalize(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) (ctrl.Result, error) {
	if !containsString(ppvpa.Finalizers, ppvpaFinalizer) {
		return ctrl.Result{}, nil
	}
	// Cascade delete via ownerReferences is enough for PRRs and the checkpoint,
	// so we only strip the finalizer here.
	ppvpa.Finalizers = removeString(ppvpa.Finalizers, ppvpaFinalizer)
	if err := r.Update(ctx, ppvpa); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// targetedPods returns the pods owned (directly or through a ReplicaSet) by
// the PP-VPA's targetRef. Only Deployments are supported here; other workload
// kinds would extend the chain-walk.
func (r *PerPodVerticalPodAutoscalerReconciler) targetedPods(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) ([]corev1.Pod, error) {
	tr := ppvpa.Spec.TargetRef
	if tr.Kind != "Deployment" {
		return nil, fmt.Errorf("unsupported targetRef kind %q", tr.Kind)
	}
	var dep appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Namespace: ppvpa.Namespace, Name: tr.Name}, &dep); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var rsList appsv1.ReplicaSetList
	if err := r.List(ctx, &rsList, client.InNamespace(ppvpa.Namespace)); err != nil {
		return nil, err
	}
	rsUIDs := map[types.UID]struct{}{}
	for _, rs := range rsList.Items {
		for _, o := range rs.OwnerReferences {
			if o.UID == dep.UID {
				rsUIDs[rs.UID] = struct{}{}
			}
		}
	}
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(ppvpa.Namespace)); err != nil {
		return nil, err
	}
	var out []corev1.Pod
	for i := range podList.Items {
		p := &podList.Items[i]
		for _, o := range p.OwnerReferences {
			if _, ok := rsUIDs[o.UID]; ok {
				out = append(out, *p)
				break
			}
		}
	}
	return out, nil
}

// reconcilePRRs enforces 1:1 PRR-per-pod. PRRs without a matching pod are deleted.
func (r *PerPodVerticalPodAutoscalerReconciler) reconcilePRRs(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler, pods []corev1.Pod) error {
	existing, err := r.listOwnedPRRs(ctx, ppvpa)
	if err != nil {
		return err
	}
	byPod := map[string]*autoscalingv1alpha1.PodResourceRecommendation{}
	for i := range existing {
		byPod[existing[i].Spec.TargetPodName] = &existing[i]
	}
	live := map[string]struct{}{}
	for i := range pods {
		live[pods[i].Name] = struct{}{}
		if _, ok := byPod[pods[i].Name]; ok {
			continue
		}
		prr := &autoscalingv1alpha1.PodResourceRecommendation{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ppvpa.Namespace,
				Name:      fmt.Sprintf("%s-%s-prr", ppvpa.Name, pods[i].Name),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: autoscalingv1alpha1.GroupVersion.String(),
						Kind:       "PerPodVerticalPodAutoscaler",
						Name:       ppvpa.Name,
						UID:        ppvpa.UID,
						Controller: ptrBool(true),
					},
				},
			},
			Spec: autoscalingv1alpha1.PodResourceRecommendationSpec{
				TargetPodName: pods[i].Name,
			},
		}
		if err := r.Create(ctx, prr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	for podName, prr := range byPod {
		if _, ok := live[podName]; ok {
			continue
		}
		if err := r.Delete(ctx, prr); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *PerPodVerticalPodAutoscalerReconciler) listOwnedPRRs(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) ([]autoscalingv1alpha1.PodResourceRecommendation, error) {
	var prrs autoscalingv1alpha1.PodResourceRecommendationList
	if err := r.List(ctx, &prrs, client.InNamespace(ppvpa.Namespace)); err != nil {
		return nil, err
	}
	out := prrs.Items[:0]
	for _, p := range prrs.Items {
		for _, o := range p.OwnerReferences {
			if o.UID == ppvpa.UID {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
}

// reconcileRecommendation runs the aggregate recommender and writes
// status.defaultRecommendation. Also writes the workload checkpoint.
func (r *PerPodVerticalPodAutoscalerReconciler) reconcileRecommendation(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) error {
	prrs, err := r.listOwnedPRRs(ctx, ppvpa)
	if err != nil {
		return err
	}
	rec, err := recommender.Recommend(prrs, ppvpa.Spec, time.Since(ppvpa.CreationTimestamp.Time))
	if err != nil {
		return err
	}
	patch := ppvpa.DeepCopy()
	patch.Status.DefaultRecommendation = rec
	return r.Status().Patch(ctx, patch, client.MergeFrom(ppvpa))
}

// reconcileReplicas keeps Deployment.spec.replicas in lock-step with
// targetReplicas + temporaryReplicas.
func (r *PerPodVerticalPodAutoscalerReconciler) reconcileReplicas(ctx context.Context, ppvpa *autoscalingv1alpha1.PerPodVerticalPodAutoscaler) error {
	tr := ppvpa.Spec.TargetRef
	if tr.Kind != "Deployment" {
		return nil
	}
	var dep appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Namespace: ppvpa.Namespace, Name: tr.Name}, &dep); err != nil {
		return client.IgnoreNotFound(err)
	}
	var target int32
	if ppvpa.Spec.TargetReplicas != nil {
		target = *ppvpa.Spec.TargetReplicas
	}
	desired := target + ppvpa.Status.TemporaryReplicas
	if desired <= 0 {
		// First reconcile: seed spec.targetReplicas + status.activeReplicas from the current deployment.
		current := int32(1)
		if dep.Spec.Replicas != nil {
			current = *dep.Spec.Replicas
		}
		if ppvpa.Spec.TargetReplicas == nil {
			specPatch := ppvpa.DeepCopy()
			specPatch.Spec.TargetReplicas = &current
			if err := r.Patch(ctx, specPatch, client.MergeFrom(ppvpa)); err != nil {
				return err
			}
		}
		statusPatch := ppvpa.DeepCopy()
		statusPatch.Status.ActiveReplicas = current
		return r.Status().Patch(ctx, statusPatch, client.MergeFrom(ppvpa))
	}
	if dep.Spec.Replicas != nil && *dep.Spec.Replicas == desired {
		return nil
	}
	depCopy := dep.DeepCopy()
	depCopy.Spec.Replicas = &desired
	return r.Patch(ctx, depCopy, client.MergeFrom(&dep))
}

func (r *PerPodVerticalPodAutoscalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.PerPodVerticalPodAutoscaler{}).
		Owns(&autoscalingv1alpha1.PodResourceRecommendation{}).
		Named("perpodverticalpodautoscaler").
		Complete(r)
}

func containsString(s []string, v string) bool {
	return slices.Contains(s, v)
}

func removeString(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func ptrBool(b bool) *bool { return &b }
