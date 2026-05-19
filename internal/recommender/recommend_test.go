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

package recommender

import (
	"testing"
	"time"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRecommendFullPipeline(t *testing.T) {
	cpuH, _ := histogram.New(histogram.DefaultCPUOptions())
	cpuH.AddSample(2.0, 1.0, time.Now())
	cpuEnc, _ := histogram.Encode(cpuH)

	memH, _ := histogram.New(histogram.DefaultMemoryOptions())
	memH.AddSample(1<<30, 1.0, time.Now())
	memEnc, _ := histogram.Encode(memH)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "test-prr", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				HistogramCheckpoint: cpuEnc,
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{ContainerName: "app"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "test-prr-2", Namespace: "default"},
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				HistogramCheckpoint: memEnc,
			},
		},
	}

	spec := autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
		RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{
			TargetPercentile:       "90.0",
			LowerBoundPercentile:   "50.0",
			UpperBoundPercentile:   "95.0",
			SafetyMarginPercentage: 15,
		},
	}

	rec, err := Recommend(prrs, spec, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		t.Fatal("expected non-nil recommendation")
	}
	if rec.PodLevelTarget == nil {
		t.Fatal("expected non-nil PodLevelTarget")
	}
	if len(rec.ContainerRecommendations) == 0 {
		t.Fatal("expected at least one container recommendation")
	}
	if rec.ContainerRecommendations[0].ContainerName != "app" {
		t.Fatalf("expected container name 'app', got %v", rec.ContainerRecommendations[0].ContainerName)
	}
}

func TestRecommendClampingToPolicy(t *testing.T) {
	cpuH, _ := histogram.New(histogram.DefaultCPUOptions())
	for i := 0; i < 100; i++ {
		cpuH.AddSample(8.0, 1.0, time.Now())
	}
	cpuEnc, _ := histogram.Encode(cpuH)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				HistogramCheckpoint: cpuEnc,
			},
		},
	}

	maxCPU := resource.MustParse("4")
	spec := autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
		RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{SafetyMarginPercentage: 15},
		ResourcePolicy: autoscalingv1alpha1.ResourcePolicy{
			PodLevelPolicies: []autoscalingv1alpha1.PodLevelPolicy{
				{
					ResourceName: corev1.ResourceCPU,
					MaxAllowed:   &maxCPU,
				},
			},
		},
	}

	rec, err := Recommend(prrs, spec, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// UpperBound should be clamped to 4 cores (4000m).
	upperCPU := rec.PodLevel.UpperBound.Cpu().MilliValue()
	if upperCPU > 4000 {
		t.Fatalf("expected CPU upper bound clamped to 4000m, got %d", upperCPU)
	}
}

func TestRecommendContainerNamesExtracted(t *testing.T) {
	cpuH, _ := histogram.New(histogram.DefaultCPUOptions())
	cpuH.AddSample(1.0, 1.0, time.Now())
	cpuEnc, _ := histogram.Encode(cpuH)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				HistogramCheckpoint: cpuEnc,
				ContainerRecommendations: []autoscalingv1alpha1.ContainerRecommendation{
					{ContainerName: "web"},
					{ContainerName: "sidecar"},
				},
			},
		},
	}

	spec := autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
		RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{SafetyMarginPercentage: 10},
	}
	rec, err := Recommend(prrs, spec, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.ContainerRecommendations) != 2 {
		t.Fatalf("expected 2 container recommendations, got %d", len(rec.ContainerRecommendations))
	}
	names := map[string]bool{}
	for _, c := range rec.ContainerRecommendations {
		names[c.ContainerName] = true
	}
	if !names["web"] || !names["sidecar"] {
		t.Fatalf("expected 'web' and 'sidecar' containers, got %v", names)
	}
}

func TestRecommendPodLevelTarget(t *testing.T) {
	memH, _ := histogram.New(histogram.DefaultMemoryOptions())
	memH.AddSample(2<<30, 1.0, time.Now())
	memEnc, _ := histogram.Encode(memH)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{
			Status: autoscalingv1alpha1.PodResourceRecommendationStatus{
				HistogramCheckpoint: memEnc,
			},
		},
	}

	spec := autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
		RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{
			TargetPercentile:       "90.0",
			SafetyMarginPercentage: 0,
		},
	}
	rec, err := Recommend(prrs, spec, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	memTarget := rec.PodLevelTarget.Memory()
	if memTarget.Value() == 0 {
		t.Fatal("expected non-zero pod-level memory target")
	}
}

func TestRecommendEmptyInput(t *testing.T) {
	spec := autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
		RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{SafetyMarginPercentage: 15},
	}
	rec, err := Recommend(nil, spec, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		t.Fatal("expected non-nil recommendation even with empty input")
	}
}

func TestRecommendMarginExcludesUncapped(t *testing.T) {
	cpuH, _ := histogram.New(histogram.DefaultCPUOptions())
	for i := 0; i < 100; i++ {
		cpuH.AddSample(2.0, 1.0, time.Now())
	}
	cpuEnc, _ := histogram.Encode(cpuH)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: cpuEnc}},
	}
	spec := autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec{
		RecommenderPolicy: autoscalingv1alpha1.RecommenderPolicy{
			TargetPercentile:       "90.0",
			SafetyMarginPercentage: 50, // Large margin to make the difference obvious.
		},
	}
	rec, err := Recommend(prrs, spec, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// UncappedTarget should be strictly less than Target (which has 50% margin).
	uncapped := rec.PodLevel.UncappedTarget.Cpu().MilliValue()
	target := rec.PodLevel.Target.Cpu().MilliValue()
	if uncapped >= target {
		t.Fatalf("UncappedTarget (%dm) should be < Target (%dm) when margin > 0", uncapped, target)
	}
}
