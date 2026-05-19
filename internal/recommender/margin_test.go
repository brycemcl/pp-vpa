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

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

func TestApplyMarginPad(t *testing.T) {
	b := Bounds{CPUTarget: 1.0, MemoryTarget: 1024}
	out := ApplyMargin(b, autoscalingv1alpha1.RecommenderPolicy{SafetyMarginPercentage: 15}, 0)
	if out.CPUTarget != 1.15 {
		t.Fatalf("expected CPU pad to 1.15, got %v", out.CPUTarget)
	}
	if out.MemoryTarget != 1024*1.15 {
		t.Fatalf("expected memory pad, got %v", out.MemoryTarget)
	}
}

func TestApplyMarginMemoryFloor(t *testing.T) {
	// Proposed target is below the observed-peak floor — must be raised.
	b := Bounds{MemoryTarget: 1000, MemoryLower: 500, MemoryUpper: 2000}
	out := ApplyMargin(b, autoscalingv1alpha1.RecommenderPolicy{SafetyMarginPercentage: 10}, 2000)
	if out.MemoryTarget < 2200 {
		t.Fatalf("memory floor not enforced: got %v, want >= 2200", out.MemoryTarget)
	}
}
