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

package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/eviction"
)

// PodResourceRecommendationReconciler watches PRR Conditions and triggers
// the appropriate eviction strategy.
type PodResourceRecommendationReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Anomaly          *eviction.AnomalyHandler
	BudgetedFallback *eviction.BudgetedHandler
}

// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=podresourcerecommendations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=podresourcerecommendations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.brycemclachlan.me,resources=podresourcerecommendations/finalizers,verbs=update

func (r *PodResourceRecommendationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var prr autoscalingv1alpha1.PodResourceRecommendation
	if err := r.Get(ctx, req.NamespacedName, &prr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	ppvpa, err := r.parentPPVPA(ctx, &prr)
	if err != nil || ppvpa == nil {
		// GC will sweep orphaned PRRs via cascade delete.
		return ctrl.Result{}, err
	}

	if hasCondition(prr.Status.Conditions, autoscalingv1alpha1.PRRConditionAnomalous) && r.Anomaly != nil {
		if err := r.Anomaly.Handle(ctx, ppvpa, &prr); err != nil {
			log.Error(err, "anomaly eviction failed")
			return ctrl.Result{Requeue: true}, nil
		}
	}
	if hasCondition(prr.Status.Conditions, autoscalingv1alpha1.PRRConditionInfeasible) && r.BudgetedFallback != nil {
		if err := r.BudgetedFallback.Handle(ctx, ppvpa, &prr); err != nil {
			log.Error(err, "budgeted fallback failed")
			return ctrl.Result{Requeue: true}, nil
		}
	}
	return ctrl.Result{}, nil
}

func (r *PodResourceRecommendationReconciler) parentPPVPA(ctx context.Context, prr *autoscalingv1alpha1.PodResourceRecommendation) (*autoscalingv1alpha1.PerPodVerticalPodAutoscaler, error) {
	for _, o := range prr.OwnerReferences {
		if o.Kind != "PerPodVerticalPodAutoscaler" {
			continue
		}
		var p autoscalingv1alpha1.PerPodVerticalPodAutoscaler
		if err := r.Get(ctx, types.NamespacedName{Namespace: prr.Namespace, Name: o.Name}, &p); err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		return &p, nil
	}
	return nil, nil
}

func hasCondition(conds []metav1.Condition, t string) bool {
	for _, c := range conds {
		if c.Type == t && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *PodResourceRecommendationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.PodResourceRecommendation{}).
		Named("podresourcerecommendation").
		Complete(r)
}
