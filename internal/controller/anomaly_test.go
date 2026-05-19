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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

func mustQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(err)
	}
	return q
}

func TestDetectAnomalies_NoRecommendation(t *testing.T) {
	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"}},
	}
	patches := DetectAnomalies(prrs, nil, 900, time.Now())
	if len(patches) != 0 {
		t.Fatalf("expected no patches when recommendation is nil, got %d", len(patches))
	}
}

func TestDetectAnomalies_WithinBounds(t *testing.T) {
	now := time.Now()
	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "1Gi",
					},
				},
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			UpperBound: corev1.ResourceList{
				corev1.ResourceMemory: mustQuantity("2Gi"),
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	// Usage is within bounds, so we expect SetAnomalousFalse but no exceed-since changes.
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	p := patches[0]
	if p.SetAnomalousFalse != true {
		t.Error("expected SetAnomalousFalse")
	}
	if p.SetAnomalousTrue {
		t.Error("did not expect SetAnomalousTrue")
	}
	if p.AnomalyExceedSince != nil {
		t.Error("did not expect AnomalyExceedSince to be set")
	}
}

func TestDetectAnomalies_ExceedsUpperBound_FirstTime(t *testing.T) {
	now := time.Now()
	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "3Gi",
					},
				},
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			UpperBound: corev1.ResourceList{
				corev1.ResourceMemory: mustQuantity("2Gi"),
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	p := patches[0]
	// First time exceeding: AnomalyExceedSince should be set, but Anomalous not yet True.
	if p.AnomalyExceedSince == nil {
		t.Fatal("expected AnomalyExceedSince to be set")
	}
	expected := metav1.NewTime(now)
	if !p.AnomalyExceedSince.Equal(&expected) {
		t.Errorf("AnomalyExceedSince = %v, want %v", p.AnomalyExceedSince, now)
	}
	if p.SetAnomalousTrue {
		t.Error("did not expect SetAnomalousTrue on first exceed")
	}
}

func TestDetectAnomalies_ExceedsTimeout(t *testing.T) {
	now := time.Now()
	exceedSince := metav1.NewTime(now.Add(-1000 * time.Second)) // 1000s ago, well over 900s timeout.

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "3Gi",
					},
				},
				AnomalyExceedSince: &exceedSince,
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			UpperBound: corev1.ResourceList{
				corev1.ResourceMemory: mustQuantity("2Gi"),
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	p := patches[0]
	if !p.SetAnomalousTrue {
		t.Error("expected SetAnomalousTrue after timeout exceeded")
	}
}

func TestDetectAnomalies_Recovery(t *testing.T) {
	now := time.Now()
	exceedSince := metav1.NewTime(now.Add(-1000 * time.Second))

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "1Gi", // Back within bounds.
					},
				},
				AnomalyExceedSince: &exceedSince,
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			UpperBound: corev1.ResourceList{
				corev1.ResourceMemory: mustQuantity("2Gi"),
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	p := patches[0]
	if !p.ClearAnomalyExceedSince {
		t.Error("expected ClearAnomalyExceedSince on recovery")
	}
	if !p.SetAnomalousFalse {
		t.Error("expected SetAnomalousFalse on recovery")
	}
	if p.SetAnomalousTrue {
		t.Error("did not expect SetAnomalousTrue on recovery")
	}
}

func TestDetectAnomalies_MultiplePRRs(t *testing.T) {
	now := time.Now()
	exceedSince := metav1.NewTime(now.Add(-1000 * time.Second))

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "3Gi",
					},
				},
				AnomalyExceedSince: &exceedSince,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-2", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "1Gi",
					},
				},
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			UpperBound: corev1.ResourceList{
				corev1.ResourceMemory: mustQuantity("2Gi"),
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}

	// prr-1 should be anomalous.
	foundAnomalous := false
	foundHealthy := false
	for _, p := range patches {
		if p.Name == "prr-1" && p.SetAnomalousTrue {
			foundAnomalous = true
		}
		if p.Name == "prr-2" && p.SetAnomalousFalse && !p.SetAnomalousTrue {
			foundHealthy = true
		}
	}
	if !foundAnomalous {
		t.Error("expected prr-1 to be flagged anomalous")
	}
	if !foundHealthy {
		t.Error("expected prr-2 to be flagged healthy")
	}
}

func TestDetectAnomalies_NotYetTimedOut(t *testing.T) {
	now := time.Now()
	exceedSince := metav1.NewTime(now.Add(-500 * time.Second)) // 500s, less than 900s timeout.

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ObservedPeak: &autoscalingv1alpha1.ObservedPeak{
					PodLevel: &autoscalingv1alpha1.ObservedPeakPodLevel{
						Memory: "3Gi",
					},
				},
				AnomalyExceedSince: &exceedSince,
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			UpperBound: corev1.ResourceList{
				corev1.ResourceMemory: mustQuantity("2Gi"),
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	if len(patches) != 0 {
		// Exceeding but not yet timed out, and AnomalyExceedSince is already set,
		// so no new changes needed.
		t.Fatalf("expected 0 patches (no new changes), got %d", len(patches))
	}
}

func TestDetectAnomalies_ContainerLevelExceed(t *testing.T) {
	now := time.Now()
	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prr-1", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{
						ContainerName: "app",
						Target: corev1.ResourceList{
							corev1.ResourceMemory: mustQuantity("3Gi"),
						},
					},
				},
			},
		},
	}
	rec := &autoscalingv1alpha1.Recommendation{
		ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
			{
				ContainerName: "app",
				UpperBound: corev1.ResourceList{
					corev1.ResourceMemory: mustQuantity("2Gi"),
				},
			},
		},
	}
	patches := DetectAnomalies(prrs, rec, 900, now)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].AnomalyExceedSince == nil {
		t.Error("expected AnomalyExceedSince for container-level exceed")
	}
}
