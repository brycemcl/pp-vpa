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

package trigger

import (
	"math"
	"testing"
)

func TestCPUScaleDownFires(t *testing.T) {
	d := EvaluateCPUScaleDown(400, 1000, 20)
	if !d.Fire {
		t.Fatal("expected CPU scale-down to fire")
	}
	if d.Reason != "buffer-drop" {
		t.Fatalf("expected reason 'buffer-drop', got %v", d.Reason)
	}
	if d.NewTarget != 400 {
		t.Fatalf("expected NewTarget=400, got %v", d.NewTarget)
	}
}

func TestCPUScaleDownNoFire(t *testing.T) {
	d := EvaluateCPUScaleDown(900, 1000, 20)
	if d.Fire {
		t.Fatal("expected no fire when utilization is above threshold")
	}
}

func TestCPUScaleDownZeroBufferDrop(t *testing.T) {
	d := EvaluateCPUScaleDown(400, 1000, 0)
	if d.Fire {
		t.Fatal("expected no fire when bufferDropPct=0")
	}
}

func TestCPUScaleDownZeroWatermark(t *testing.T) {
	d := EvaluateCPUScaleDown(100, 0, 20)
	if d.Fire {
		t.Fatal("expected no fire when watermark=0")
	}
}

func TestCPUScaleDownAtBoundary(t *testing.T) {
	// threshold = 1000 * (1 - 20/100) = 800. utilization = 800 → fires.
	d := EvaluateCPUScaleDown(800, 1000, 20)
	if !d.Fire {
		t.Fatal("expected fire at exact boundary")
	}
}

func TestMemoryScaleDownNormal(t *testing.T) {
	// Proposed 2048 is above the peak floor of 1024*1.0 = 1024, so it passes through.
	d := EvaluateMemoryScaleDown(2048, 1024, 0)
	if !d.Fire {
		t.Fatal("expected memory scale-down to fire")
	}
	if d.NewTarget != 2048 {
		t.Fatalf("expected NewTarget=2048, got %v", d.NewTarget)
	}
}

func TestMemoryScaleDownCappedByPeakFloor(t *testing.T) {
	// Proposed 512, but floor = 1000 * 1.15 = 1150 → capped.
	d := EvaluateMemoryScaleDown(512, 1000, 15)
	if !d.Fire {
		t.Fatal("expected memory scale-down to fire")
	}
	expected := 1000 * 1.15
	if math.Abs(d.NewTarget-expected) > 0.01 {
		t.Fatalf("expected NewTarget=%v (peak floor), got %v", expected, d.NewTarget)
	}
}

func TestMemoryScaleDownNoPeakData(t *testing.T) {
	d := EvaluateMemoryScaleDown(512, 0, 15)
	if !d.Fire {
		t.Fatal("expected fire when no peak data")
	}
	if d.NewTarget != 512 {
		t.Fatalf("expected NewTarget=512 (no peak constraint), got %v", d.NewTarget)
	}
}

func TestMemoryScaleDownZeroProposed(t *testing.T) {
	d := EvaluateMemoryScaleDown(0, 0, 15)
	if d.Fire {
		t.Fatal("expected no fire when proposed=0 and no peak")
	}
}

func TestMemoryScaleDownAtPeak(t *testing.T) {
	// Proposed = 1000, floor = 1000 * 1.15 = 1150 → capped to 1150.
	d := EvaluateMemoryScaleDown(1000, 1000, 15)
	expected := 1000 * 1.15
	if math.Abs(d.NewTarget-expected) > 0.01 {
		t.Fatalf("expected NewTarget=%v, got %v", expected, d.NewTarget)
	}
}
