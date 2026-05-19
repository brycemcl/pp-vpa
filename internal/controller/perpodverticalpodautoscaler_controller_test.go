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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

var _ = Describe("PerPodVerticalPodAutoscaler Controller", func() {
	ctx := context.Background()

	Context("PRR-per-pod invariant", func() {
		const ppvpaName = "test-prr-invariant"
		const deployName = "test-deploy-prr"
		const ns = "default"

		var deploy *appsv1.Deployment

		BeforeEach(func() {
			deploy = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptrInt32(2),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": deployName}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": deployName}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "app",
								Image: "registry.k8s.io/pause:3.9",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
							}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())
		})

		AfterEach(func() {
			cleanupPPVPA(ctx, ppvpaName, ns)
			Expect(k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns}})).To(Succeed())
		})

		It("should create a PPVPA and reconcile without error", func() {
			ppvpa := makePPVPA(ppvpaName, ns, deployName)
			Expect(k8sClient.Create(ctx, ppvpa)).To(Succeed())

			r := &PerPodVerticalPodAutoscalerReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: ppvpaName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added.
			updated := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ppvpaName, Namespace: ns}, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement("autoscaling.brycemclachlan.me/ppvpa-finalizer"))
		})
	})

	Context("PPVPA not found", func() {
		It("should return without error", func() {
			r := &PerPodVerticalPodAutoscalerReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Recommendation computation", func() {
		const ppvpaName = "test-recommend"
		const deployName = "test-deploy-rec"
		const ns = "default"

		AfterEach(func() {
			cleanupPPVPA(ctx, ppvpaName, ns)
			Expect(k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns}})).To(Succeed())
		})

		It("should populate status.defaultRecommendation after reconcile", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptrInt32(1),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": deployName}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": deployName}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "registry.k8s.io/pause:3.9"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			ppvpa := makePPVPA(ppvpaName, ns, deployName)
			Expect(k8sClient.Create(ctx, ppvpa)).To(Succeed())

			r := &PerPodVerticalPodAutoscalerReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: ppvpaName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())

			updated := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ppvpaName, Namespace: ns}, updated)).To(Succeed())
			// Even with no PRR data, the recommendation should be non-nil (zero values).
			Expect(updated.Status.DefaultRecommendation).NotTo(BeNil())
		})
	})

	Context("Finalizer management", func() {
		const ns = "default"

		It("should add finalizer on first reconcile", func() {
			const ppvpaName = "test-finalizer-add"
			const deployName = "test-deploy-fin-add"
			ppvpa := makePPVPA(ppvpaName, ns, deployName)
			Expect(k8sClient.Create(ctx, ppvpa)).To(Succeed())
			defer cleanupPPVPA(ctx, ppvpaName, ns)

			r := &PerPodVerticalPodAutoscalerReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: ppvpaName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())

			updated := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ppvpaName, Namespace: ns}, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement("autoscaling.brycemclachlan.me/ppvpa-finalizer"))
		})

		It("should remove finalizer when PPVPA is being deleted", func() {
			const ppvpaName = "test-finalizer-remove"
			const deployName = "test-deploy-fin-remove"
			ppvpa := makePPVPA(ppvpaName, ns, deployName)
			Expect(k8sClient.Create(ctx, ppvpa)).To(Succeed())

			r := &PerPodVerticalPodAutoscalerReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: ppvpaName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())

			// Delete the PPVPA.
			Expect(k8sClient.Delete(ctx, &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: ppvpaName, Namespace: ns},
			})).To(Succeed())

			// Reconcile should strip the finalizer.
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: ppvpaName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())

			// PPVPA should be gone after finalizer removal.
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ppvpaName, Namespace: ns}, &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})

func makePPVPA(name, ns, deployName string) *autoscalingv1alpha1.PerPodVerticalPodAutoscaler {
	return &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: deployName},
			UpdatePolicy: autoscalingv1alpha1.UpdatePolicy{
				UpdateMode: autoscalingv1alpha1.UpdateModeInPlace,
			},
			RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{
				SafetyMarginPercentage: 15,
			},
		},
	}
}

func cleanupPPVPA(ctx context.Context, name, ns string) {
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, ppvpa); err == nil {
		_ = k8sClient.Delete(ctx, ppvpa)
	}
}

func ptrInt32(v int32) *int32 { return &v }
