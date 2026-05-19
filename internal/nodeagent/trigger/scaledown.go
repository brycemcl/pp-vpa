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

// ScaleDownDecision is the output of EvaluateScaleDown.
type ScaleDownDecision struct {
	Fire      bool
	NewTarget float64
	Reason    string
}

// EvaluateCPUScaleDown fires when utilization has dropped by bufferDropPct
// relative to the high-watermark. Returns the proposed new target equal to
// utilization (the recommender's percentile will then apply on top).
func EvaluateCPUScaleDown(utilization, watermark float64, bufferDropPct float64) ScaleDownDecision {
	if watermark <= 0 || bufferDropPct <= 0 {
		return ScaleDownDecision{}
	}
	threshold := watermark * (1.0 - bufferDropPct/100.0)
	if utilization <= threshold {
		return ScaleDownDecision{Fire: true, NewTarget: utilization, Reason: "buffer-drop"}
	}
	return ScaleDownDecision{}
}

// EvaluateMemoryScaleDown is strictly bounded by the observed peak plus the
// safety margin. Returns the proposed memory target capped by the peak floor.
func EvaluateMemoryScaleDown(proposed, observedPeak, marginPct float64) ScaleDownDecision {
	if observedPeak <= 0 {
		return ScaleDownDecision{Fire: proposed > 0, NewTarget: proposed, Reason: "no-peak-data"}
	}
	floor := observedPeak * (1.0 + marginPct/100.0)
	if proposed < floor {
		proposed = floor
	}
	return ScaleDownDecision{Fire: true, NewTarget: proposed, Reason: "peak-bound"}
}
