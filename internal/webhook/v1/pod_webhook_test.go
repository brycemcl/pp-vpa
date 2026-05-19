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

package v1

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/eviction"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = autoscalingv1alpha1.AddToScheme(s)
	return s
}

func makeResources(cpu, mem string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
}

func TestDefaultNilPod(t *testing.T) {
	d := &PodCustomDefaulter{}
	if err := d.Default(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultSurgePodSkipped(t *testing.T) {
	scheme := newScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	d := &PodCustomDefaulter{Client: fc}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "web-pod",
			Namespace:   "default",
			Annotations: map[string]string{eviction.AnnotationSurgePod: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}
	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	// Should not have mutated the container resources.
	if len(pod.Spec.Containers[0].Resources.Requests) != 0 {
		t.Fatal("surge pod should not be mutated")
	}
}

func TestDefaultNoPPVPA(t *testing.T) {
	scheme := newScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	d := &PodCustomDefaulter{Client: fc}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	if len(pod.Spec.Containers[0].Resources.Requests) != 0 {
		t.Fatal("expected no mutation without PPVPA")
	}
}

func TestDefaultInjectsTarget(t *testing.T) {
	scheme := newScheme()
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ppvpa", Namespace: "default", UID: "ppvpa-uid"},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "web-deploy"},
		},
		Status: autoscalingv1alpha1.PerPodVerticalPodAutoscalerStatus{
			DefaultRecommendation: &autoscalingv1alpha1.Recommendation{
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{
						ContainerName: "app",
						Target:        makeResources("500m", "512Mi"),
						UpperBound:    makeResources("1", "1Gi"),
					},
				},
			},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web-deploy"},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-rs"},
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ppvpa, rs).Build()
	d := &PodCustomDefaulter{Client: fc}

	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	cpu := pod.Spec.Containers[0].Resources.Requests.Cpu().MilliValue()
	if cpu != 500 {
		t.Fatalf("expected target CPU 500m, got %d", cpu)
	}
}

func TestDefaultInjectsUpperWhenTemporaryReplicas(t *testing.T) {
	scheme := newScheme()
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ppvpa", Namespace: "default", UID: "ppvpa-uid"},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "web-deploy"},
		},
		Status: autoscalingv1alpha1.PerPodVerticalPodAutoscalerStatus{
			TemporaryReplicas: 1,
			DefaultRecommendation: &autoscalingv1alpha1.Recommendation{
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{
						ContainerName: "app",
						Target:        makeResources("500m", "512Mi"),
						UpperBound:    makeResources("1", "1Gi"),
					},
				},
			},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web-deploy"},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-rs"},
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ppvpa, rs).Build()
	d := &PodCustomDefaulter{Client: fc}

	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	cpu := pod.Spec.Containers[0].Resources.Requests.Cpu().MilliValue()
	if cpu != 1000 {
		t.Fatalf("expected upper CPU 1000m, got %d", cpu)
	}
}

func TestDefaultInjectsUpperWhenSiblingInfeasible(t *testing.T) {
	scheme := newScheme()
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ppvpa", Namespace: "default", UID: "ppvpa-uid"},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "web-deploy"},
		},
		Status: autoscalingv1alpha1.PerPodVerticalPodAutoscalerStatus{
			DefaultRecommendation: &autoscalingv1alpha1.Recommendation{
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{
						ContainerName: "app",
						Target:        makeResources("500m", "512Mi"),
						UpperBound:    makeResources("1", "1Gi"),
					},
				},
			},
		},
	}
	siblingPRR := &autoscalingv1alpha1.PodResourceRecommendation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sibling-prr",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "PerPodVerticalPodAutoscaler", Name: "test-ppvpa", UID: "ppvpa-uid"},
			},
		},
		Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
			Conditions: []metav1.Condition{
				{Type: autoscalingv1alpha1.PRRConditionInfeasible, Status: metav1.ConditionTrue},
			},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web-deploy"},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-rs"},
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ppvpa, siblingPRR, rs).Build()
	d := &PodCustomDefaulter{Client: fc}

	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	cpu := pod.Spec.Containers[0].Resources.Requests.Cpu().MilliValue()
	if cpu != 1000 {
		t.Fatalf("expected upper CPU 1000m (sibling infeasible), got %d", cpu)
	}
}

func TestDefaultInjectsUpperWhenSiblingAnomalous(t *testing.T) {
	scheme := newScheme()
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ppvpa", Namespace: "default", UID: "ppvpa-uid"},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "web-deploy"},
		},
		Status: autoscalingv1alpha1.PerPodVerticalPodAutoscalerStatus{
			DefaultRecommendation: &autoscalingv1alpha1.Recommendation{
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{
						ContainerName: "app",
						Target:        makeResources("500m", "512Mi"),
						UpperBound:    makeResources("1", "1Gi"),
					},
				},
			},
		},
	}
	siblingPRR := &autoscalingv1alpha1.PodResourceRecommendation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sibling-prr",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "PerPodVerticalPodAutoscaler", Name: "test-ppvpa", UID: "ppvpa-uid"},
			},
		},
		Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
			Conditions: []metav1.Condition{
				{Type: autoscalingv1alpha1.PRRConditionAnomalous, Status: metav1.ConditionTrue},
			},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web-deploy"},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-rs"},
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ppvpa, siblingPRR, rs).Build()
	d := &PodCustomDefaulter{Client: fc}

	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	cpu := pod.Spec.Containers[0].Resources.Requests.Cpu().MilliValue()
	if cpu != 1000 {
		t.Fatalf("expected upper CPU 1000m (sibling anomalous), got %d", cpu)
	}
}

func TestDefaultNoRecommendation(t *testing.T) {
	scheme := newScheme()
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ppvpa", Namespace: "default"},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "web-deploy"},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web-deploy"},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-rs"},
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ppvpa, rs).Build()
	d := &PodCustomDefaulter{Client: fc}

	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	if len(pod.Spec.Containers[0].Resources.Requests) != 0 {
		t.Fatal("expected no mutation without recommendation")
	}
}

func TestDefaultContainerMatching(t *testing.T) {
	scheme := newScheme()
	ppvpa := &autoscalingv1alpha1.PerPodVerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ppvpa", Namespace: "default"},
		Spec: autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
			TargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "web-deploy"},
		},
		Status: autoscalingv1alpha1.PerPodVerticalPodAutoscalerStatus{
			DefaultRecommendation: &autoscalingv1alpha1.Recommendation{
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{ContainerName: "app", Target: makeResources("500m", "512Mi")},
				},
			},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web-deploy"},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "web-rs"},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ppvpa, rs).Build()
	d := &PodCustomDefaulter{Client: fc}

	if err := d.Default(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	// Only "app" should get resources; "sidecar" has no recommendation.
	if len(pod.Spec.Containers[0].Resources.Requests) == 0 {
		t.Fatal("expected 'app' container to get resources")
	}
	if len(pod.Spec.Containers[1].Resources.Requests) != 0 {
		t.Fatal("expected 'sidecar' container to remain unmodified")
	}
}

// Verify unused imports don't break compilation.
var _ types.UID
var _ intstr.IntOrString
var _ client.Client
