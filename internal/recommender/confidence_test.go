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
	"math"
	"testing"
	"time"
)

func TestConfidenceYoungWorkload(t *testing.T) {
	c := ConfidenceMultipliers(1 * time.Hour)
	if c.LowerMultiplier >= 1.0 {
		t.Fatalf("young workload lower multiplier should be < 1.0, got %v", c.LowerMultiplier)
	}
	if c.UpperMultiplier <= 1.0 {
		t.Fatalf("young workload upper multiplier should be > 1.0, got %v", c.UpperMultiplier)
	}
}

func TestConfidenceMatureWorkload(t *testing.T) {
	c := ConfidenceMultipliers(30 * 24 * time.Hour)
	if math.Abs(c.LowerMultiplier-1.0) > 0.05 {
		t.Fatalf("mature workload lower multiplier should be ~1.0, got %v", c.LowerMultiplier)
	}
	if math.Abs(c.UpperMultiplier-1.0) > 0.05 {
		t.Fatalf("mature workload upper multiplier should be ~1.0, got %v", c.UpperMultiplier)
	}
}

func TestConfidenceZeroAge(t *testing.T) {
	c := ConfidenceMultipliers(0)
	if math.IsNaN(c.LowerMultiplier) || math.IsInf(c.LowerMultiplier, 0) {
		t.Fatalf("zero age lower multiplier should be finite, got %v", c.LowerMultiplier)
	}
	if math.IsNaN(c.UpperMultiplier) || math.IsInf(c.UpperMultiplier, 0) {
		t.Fatalf("zero age upper multiplier should be finite, got %v", c.UpperMultiplier)
	}
}

func TestConfidenceNegativeAge(t *testing.T) {
	c := ConfidenceMultipliers(-1 * time.Hour)
	if math.IsNaN(c.LowerMultiplier) || math.IsInf(c.LowerMultiplier, 0) {
		t.Fatalf("negative age lower multiplier should be finite, got %v", c.LowerMultiplier)
	}
	if math.IsNaN(c.UpperMultiplier) || math.IsInf(c.UpperMultiplier, 0) {
		t.Fatalf("negative age upper multiplier should be finite, got %v", c.UpperMultiplier)
	}
}

func TestConfidenceDecreasesOverTime(t *testing.T) {
	c1 := ConfidenceMultipliers(1 * time.Hour)
	c7 := ConfidenceMultipliers(7 * 24 * time.Hour)
	// As workload ages, multipliers converge toward 1.0.
	if c1.LowerMultiplier > c7.LowerMultiplier {
		t.Fatalf("lower multiplier should increase toward 1.0 over time: 1h=%v, 7d=%v", c1.LowerMultiplier, c7.LowerMultiplier)
	}
	if c1.UpperMultiplier < c7.UpperMultiplier {
		t.Fatalf("upper multiplier should decrease toward 1.0 over time: 1h=%v, 7d=%v", c1.UpperMultiplier, c7.UpperMultiplier)
	}
}

func TestApplyConfidenceBounds(t *testing.T) {
	b := Bounds{
		CPULower: 1.0, CPUTarget: 2.0, CPUUncapped: 2.1, CPUUpper: 3.0,
		MemoryLower: 1024, MemoryTarget: 2048, MemoryUncapped: 2100, MemoryUpper: 3072,
	}
	c := Confidence{LowerMultiplier: 0.8, UpperMultiplier: 1.2}
	out := ApplyConfidence(b, c)

	if out.CPULower != 0.8 {
		t.Fatalf("expected CPULower=0.8, got %v", out.CPULower)
	}
	if math.Abs(out.CPUUpper-3.6) > 1e-9 {
		t.Fatalf("expected CPUUpper≈3.6, got %v", out.CPUUpper)
	}
	if out.CPUTarget != 2.0 {
		t.Fatalf("expected CPUTarget unchanged=2.0, got %v", out.CPUTarget)
	}
	if math.Abs(out.MemoryLower-819.2) > 1e-9 {
		t.Fatalf("expected MemoryLower≈819.2, got %v", out.MemoryLower)
	}
	if math.Abs(out.MemoryUpper-3686.4) > 1e-9 {
		t.Fatalf("expected MemoryUpper≈3686.4, got %v", out.MemoryUpper)
	}
}

func TestApplyConfidencePreservesUncapped(t *testing.T) {
	b := Bounds{CPUUncapped: 2.5, MemoryUncapped: 3000}
	c := Confidence{LowerMultiplier: 0.5, UpperMultiplier: 2.0}
	out := ApplyConfidence(b, c)

	if out.CPUUncapped != 2.5 {
		t.Fatalf("expected CPUUncapped unchanged=2.5, got %v", out.CPUUncapped)
	}
	if out.MemoryUncapped != 3000 {
		t.Fatalf("expected MemoryUncapped unchanged=3000, got %v", out.MemoryUncapped)
	}
}
