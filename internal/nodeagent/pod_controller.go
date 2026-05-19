/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodeagent

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

// PodReconciler watches pods scheduled to the local node and registers them
// with the Agent for PSI sampling and resize triggering.
type PodReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Agent    *Agent
	NodeName string
}

// SetupWithManager registers the reconciler with the controller-runtime manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return r.isLocalPod(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return r.isLocalPod(e.ObjectNew)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return r.isLocalPod(e.Object)
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return r.isLocalPod(e.Object)
			},
		}).
		Complete(r)
}

func (r *PodReconciler) isLocalPod(obj client.Object) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false
	}
	return pod.Spec.NodeName == r.NodeName
}

// Reconcile handles pod create/update/delete events for the local node.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.Client.Get(ctx, req.NamespacedName, &pod); err != nil {
		r.Agent.ForgetPod(string(types.UID(req.Name)))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if pod.DeletionTimestamp != nil {
		r.Agent.ForgetPod(string(pod.UID))
		return ctrl.Result{}, nil
	}

	if pod.Spec.NodeName != r.NodeName {
		return ctrl.Result{}, nil
	}

	prr, thresholds, err := r.resolvePRR(ctx, pod)
	if err != nil {
		return ctrl.Result{}, err
	}
	if prr == nil {
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.Agent.EnsurePod(ctx, pod, *prr, thresholds); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure pod: %w", err)
	}
	return ctrl.Result{}, nil
}

// resolvePRR finds the PRR targeting this pod and resolves the owning PPVPA
// to extract scaling thresholds.
func (r *PodReconciler) resolvePRR(ctx context.Context, pod corev1.Pod) (*autoscalingv1alpha1.PodResourceRecommendation, autoscalingv1alpha1.ScalingThresholds, error) {
	var thresholds autoscalingv1alpha1.ScalingThresholds

	var prrList autoscalingv1alpha1.PodResourceRecommendationList
	if err := r.Client.List(ctx, &prrList, client.InNamespace(pod.Namespace)); err != nil {
		return nil, thresholds, fmt.Errorf("list prrs: %w", err)
	}

	for i := range prrList.Items {
		if prrList.Items[i].Spec.TargetPodName == pod.Name {
			prr := &prrList.Items[i]
			thresholds = r.resolveThresholds(ctx, prr)
			return prr, thresholds, nil
		}
	}
	return nil, thresholds, nil
}

// resolveThresholds walks the PRR's owner references to find the owning PPVPA
// and extracts scaling thresholds.
func (r *PodReconciler) resolveThresholds(ctx context.Context, prr *autoscalingv1alpha1.PodResourceRecommendation) autoscalingv1alpha1.ScalingThresholds {
	for _, ref := range prr.OwnerReferences {
		if ref.Kind != "PerPodVerticalPodAutoscaler" {
			continue
		}
		var ppvpa autoscalingv1alpha1.PerPodVerticalPodAutoscaler
		if err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: prr.Namespace,
			Name:      ref.Name,
		}, &ppvpa); err != nil {
			continue
		}
		return ppvpa.Spec.ScalingThresholds
	}
	return autoscalingv1alpha1.ScalingThresholds{}
}
