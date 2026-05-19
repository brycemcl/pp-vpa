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

func TestComputeBoundsDefaultPercentiles(t *testing.T) {
	wh := mustWorkloadHistograms(t)
	// Add CPU samples centered around 2 cores.
	for i := 0; i < 100; i++ {
		wh.CPU.AddSample(2.0, 1.0, time.Now())
	}
	// Add memory samples centered around 1 GiB.
	for i := 0; i < 100; i++ {
		wh.Memory.AddSample(1<<30, 1.0, time.Now())
	}

	b := ComputeBounds(wh, autoscalingv1alpha1.RecommenderPolicy{
		LowerBoundPercentile: "50.0",
		TargetPercentile:     "90.0",
		UpperBoundPercentile: "95.0",
	})

	if b.CPULower > b.CPUTarget {
		t.Fatalf("expected CPULower <= CPUTarget, got %v > %v", b.CPULower, b.CPUTarget)
	}
	if b.CPUTarget > b.CPUUpper {
		t.Fatalf("expected CPUTarget <= CPUUpper, got %v > %v", b.CPUTarget, b.CPUUpper)
	}
	if b.MemoryLower > b.MemoryTarget {
		t.Fatalf("expected MemoryLower <= MemoryTarget, got %v > %v", b.MemoryLower, b.MemoryTarget)
	}
	if b.MemoryTarget > b.MemoryUpper {
		t.Fatalf("expected MemoryTarget <= MemoryUpper, got %v > %v", b.MemoryTarget, b.MemoryUpper)
	}
}

func TestComputeBoundsEmptyHistogram(t *testing.T) {
	wh := mustWorkloadHistograms(t)
	b := ComputeBounds(wh, autoscalingv1alpha1.RecommenderPolicy{})

	if b.CPULower != 0 || b.CPUTarget != 0 || b.CPUUpper != 0 {
		t.Fatalf("expected zero CPU bounds for empty histogram, got lower=%v target=%v upper=%v", b.CPULower, b.CPUTarget, b.CPUUpper)
	}
	if b.MemoryLower != 0 || b.MemoryTarget != 0 || b.MemoryUpper != 0 {
		t.Fatalf("expected zero memory bounds for empty histogram, got lower=%v target=%v upper=%v", b.MemoryLower, b.MemoryTarget, b.MemoryUpper)
	}
}

func TestComputeBoundsCustomPercentiles(t *testing.T) {
	wh := mustWorkloadHistograms(t)
	for i := 0; i < 100; i++ {
		wh.CPU.AddSample(2.0, 1.0, time.Now())
	}

	bDefault := ComputeBounds(wh, autoscalingv1alpha1.RecommenderPolicy{
		LowerBoundPercentile: "50.0",
		TargetPercentile:     "90.0",
		UpperBoundPercentile: "95.0",
	})
	bCustom := ComputeBounds(wh, autoscalingv1alpha1.RecommenderPolicy{
		LowerBoundPercentile: "10.0",
		TargetPercentile:     "50.0",
		UpperBoundPercentile: "99.0",
	})

	// Wider percentiles should give wider bounds.
	if bCustom.CPULower > bDefault.CPULower {
		t.Fatalf("p10 lower should be <= p50 lower, got %v > %v", bCustom.CPULower, bDefault.CPULower)
	}
	if bCustom.CPUUpper < bDefault.CPUUpper {
		t.Fatalf("p99 upper should be >= p95 upper, got %v < %v", bCustom.CPUUpper, bDefault.CPUUpper)
	}
}

func TestComputeBoundsUncappedEqualsTarget(t *testing.T) {
	wh := mustWorkloadHistograms(t)
	for i := 0; i < 100; i++ {
		wh.CPU.AddSample(2.0, 1.0, time.Now())
	}
	b := ComputeBounds(wh, autoscalingv1alpha1.RecommenderPolicy{
		TargetPercentile: "90.0",
	})
	// UncappedTarget is the pure percentile without margin, so it equals Target at this stage.
	if b.CPUUncapped != b.CPUTarget {
		t.Fatalf("expected UncappedTarget == Target before margin, got %v != %v", b.CPUUncapped, b.CPUTarget)
	}
}

func TestComputeBoundsInvalidPercentileFallsBack(t *testing.T) {
	wh := mustWorkloadHistograms(t)
	for i := 0; i < 100; i++ {
		wh.CPU.AddSample(2.0, 1.0, time.Now())
	}
	// Invalid percentile string should fall back to defaults.
	b := ComputeBounds(wh, autoscalingv1alpha1.RecommenderPolicy{
		TargetPercentile:     "not-a-number",
		LowerBoundPercentile: "",
		UpperBoundPercentile: "",
	})
	if b.CPUTarget == 0 {
		t.Fatal("expected non-zero target with fallback percentile")
	}
}

func mustWorkloadHistograms(t *testing.T) *WorkloadHistograms {
	t.Helper()
	cpu, err := histogram.New(histogram.DefaultCPUOptions())
	if err != nil {
		t.Fatal(err)
	}
	mem, err := histogram.New(histogram.DefaultMemoryOptions())
	if err != nil {
		t.Fatal(err)
	}
	return &WorkloadHistograms{CPU: cpu, Memory: mem}
}
