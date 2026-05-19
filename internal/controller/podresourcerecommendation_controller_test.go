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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/eviction"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

var _ = Describe("PodResourceRecommendation Controller", func() {
	ctx := context.Background()

	Context("PRR not found", func() {
		It("should return without error", func() {
			r := &PodResourceRecommendationReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				Anomaly:          &eviction.AnomalyHandler{Client: k8sClient},
				BudgetedFallback: &eviction.BudgetedHandler{Client: k8sClient},
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("PRR with no conditions", func() {
		const prrName = "test-prr-nocond"
		const ppvpaName = "test-ppvpa-prr-nocond"
		const ns = "default"

		BeforeEach(func() {
			ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: ppvpaName, Namespace: ns},
				Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
					TargetRef: autoscalingv1.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "deploy"},
				},
			}
			Expect(k8sClient.Create(ctx, ppvpa)).To(Succeed())

			// Fetch to get the server-assigned UID.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ppvpaName, Namespace: ns}, ppvpa)).To(Succeed())

			prr := &autoscalingv1alpha1.PodResourceRecommendation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prrName,
					Namespace: ns,
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "PerPodVerticalPodAutoscaler", Name: ppvpaName, APIVersion: "autoscaling.brycemclachlan.me/v1alpha1", UID: ppvpa.UID},
					},
				},
				Spec: autoscalingv1alpha1.PodResourceRecommendationSpec{TargetPodName: "test-pod"},
			}
			Expect(k8sClient.Create(ctx, prr)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &autoscalingv1alpha1.PodResourceRecommendation{ObjectMeta: metav1.ObjectMeta{Name: prrName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: ppvpaName, Namespace: ns}})
		})

		It("should reconcile without error when no conditions are set", func() {
			r := &PodResourceRecommendationReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				Anomaly:          &eviction.AnomalyHandler{Client: k8sClient},
				BudgetedFallback: &eviction.BudgetedHandler{Client: k8sClient},
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: prrName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("PRR with Anomalous condition", func() {
		const prrName = "test-prr-anomalous"
		const ppvpaName = "test-ppvpa-anomalous"
		const ns = "default"

		BeforeEach(func() {
			ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: ppvpaName, Namespace: ns},
				Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
					TargetRef: autoscalingv1.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "deploy"},
				},
			}
			Expect(k8sClient.Create(ctx, ppvpa)).To(Succeed())

			// Fetch to get the server-assigned UID.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ppvpaName, Namespace: ns}, ppvpa)).To(Succeed())

			prr := &autoscalingv1alpha1.PodResourceRecommendation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prrName,
					Namespace: ns,
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "PerPodVerticalPodAutoscaler", Name: ppvpaName, APIVersion: "autoscaling.brycemclachlan.me/v1alpha1", UID: ppvpa.UID},
					},
				},
				Spec: autoscalingv1alpha1.PodResourceRecommendationSpec{TargetPodName: "test-pod"},
			}
			Expect(k8sClient.Create(ctx, prr)).To(Succeed())

			// Re-fetch to get the server-assigned ResourceVersion before status update.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: prrName, Namespace: ns}, prr)).To(Succeed())

			// Set Anomalous condition with required fields.
			prr.Status.Conditions = []metav1.Condition{
				{
					Type:               autoscalingv1alpha1.PRRConditionAnomalous,
					Status:             metav1.ConditionTrue,
					Reason:             "TestAnomaly",
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, prr)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &autoscalingv1alpha1.PodResourceRecommendation{ObjectMeta: metav1.ObjectMeta{Name: prrName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: ppvpaName, Namespace: ns}})
		})

		It("should reconcile and attempt anomaly eviction", func() {
			r := &PodResourceRecommendationReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				Anomaly:          &eviction.AnomalyHandler{Client: k8sClient},
				BudgetedFallback: &eviction.BudgetedHandler{Client: k8sClient},
			}
			// The reconcile should not error even though the pod doesn't exist.
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: prrName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
