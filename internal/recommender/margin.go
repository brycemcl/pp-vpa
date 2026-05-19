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
	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

// ApplyMargin adds the safetyMarginPercentage as a static pad to all bounds
// and enforces the minimum memory peak floor:
//
//	memoryBound = max(memoryBound, observedMemoryPeak * (1 + margin))
//
// observedMemoryPeak is the highest observedPeak across all child PRRs.
func ApplyMargin(b Bounds, p autoscalingv1alpha1.RecommenderPolicy, observedMemoryPeak float64) Bounds {
	pct := float64(p.SafetyMarginPercentage)
	if pct < 0 {
		pct = 0
	}
	factor := 1.0 + pct/100.0

	pad := func(v float64) float64 { return v * factor }

	out := Bounds{
		CPULower:       pad(b.CPULower),
		CPUTarget:      pad(b.CPUTarget),
		CPUUncapped:    b.CPUUncapped,
		CPUUpper:       pad(b.CPUUpper),
		MemoryLower:    pad(b.MemoryLower),
		MemoryTarget:   pad(b.MemoryTarget),
		MemoryUncapped: b.MemoryUncapped,
		MemoryUpper:    pad(b.MemoryUpper),
	}

	// Memory peak floor: never recommend below observedPeak * (1 + margin).
	if observedMemoryPeak > 0 {
		floor := observedMemoryPeak * factor
		if out.MemoryTarget < floor {
			out.MemoryTarget = floor
		}
		if out.MemoryLower < floor {
			out.MemoryLower = floor
		}
		if out.MemoryUpper < floor {
			out.MemoryUpper = floor
		}
	}
	return out
}
