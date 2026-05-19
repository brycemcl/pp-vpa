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
)

func TestAggregateSinglePRR(t *testing.T) {
	h, err := histogram.New(histogram.DefaultCPUOptions())
	if err != nil {
		t.Fatal(err)
	}
	h.AddSample(2.0, 1.0, time.Now())
	encoded, err := histogram.Encode(h)
	if err != nil {
		t.Fatal(err)
	}

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: encoded}},
	}
	wh, err := Aggregate(prrs)
	if err != nil {
		t.Fatal(err)
	}
	if wh.CPU.TotalWeight() == 0 {
		t.Fatal("expected non-zero CPU weight after aggregating one PRR")
	}
}

func TestAggregateMultiplePRRs(t *testing.T) {
	h1, _ := histogram.New(histogram.DefaultCPUOptions())
	h1.AddSample(2.0, 1.0, time.Now())
	enc1, _ := histogram.Encode(h1)

	h2, _ := histogram.New(histogram.DefaultCPUOptions())
	h2.AddSample(4.0, 1.0, time.Now())
	enc2, _ := histogram.Encode(h2)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: enc1}},
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: enc2}},
	}
	wh, err := Aggregate(prrs)
	if err != nil {
		t.Fatal(err)
	}
	if wh.CPU.TotalWeight() < 1.5 {
		t.Fatalf("expected merged CPU weight >= 1.5, got %v", wh.CPU.TotalWeight())
	}
}

func TestAggregateCorruptCheckpointSkipped(t *testing.T) {
	h, _ := histogram.New(histogram.DefaultCPUOptions())
	h.AddSample(2.0, 1.0, time.Now())
	valid, _ := histogram.Encode(h)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: valid}},
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: "not-valid-base64!!!"}},
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: ""}},
	}
	wh, err := Aggregate(prrs)
	if err != nil {
		t.Fatal(err)
	}
	// Should have weight from the single valid PRR only.
	if wh.CPU.TotalWeight() == 0 {
		t.Fatal("expected non-zero CPU weight from valid PRR")
	}
}

func TestAggregateEmptyPRRList(t *testing.T) {
	wh, err := Aggregate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if wh.CPU.TotalWeight() != 0 {
		t.Fatalf("expected zero CPU weight for empty PRR list, got %v", wh.CPU.TotalWeight())
	}
	if wh.Memory.TotalWeight() != 0 {
		t.Fatalf("expected zero memory weight for empty PRR list, got %v", wh.Memory.TotalWeight())
	}
}

func TestAggregateCPUVsMemoryHeuristic(t *testing.T) {
	// CPU histogram: MaxValue < 1GiB → should merge into CPU aggregate.
	cpuH, _ := histogram.New(histogram.DefaultCPUOptions())
	cpuH.AddSample(2.0, 1.0, time.Now())
	cpuEnc, _ := histogram.Encode(cpuH)

	// Memory histogram: MaxValue >= 1GiB → should merge into memory aggregate.
	memH, _ := histogram.New(histogram.DefaultMemoryOptions())
	memH.AddSample(1<<30, 1.0, time.Now())
	memEnc, _ := histogram.Encode(memH)

	prrs := []autoscalingv1alpha1.PodResourceRecommendation{
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: cpuEnc}},
		{Status: autoscalingv1alpha1.PodResourceRecommendationStatus{HistogramCheckpoint: memEnc}},
	}
	wh, err := Aggregate(prrs)
	if err != nil {
		t.Fatal(err)
	}
	if wh.CPU.TotalWeight() == 0 {
		t.Fatal("expected CPU histogram to receive CPU samples")
	}
	if wh.Memory.TotalWeight() == 0 {
		t.Fatal("expected memory histogram to receive memory samples")
	}
}
