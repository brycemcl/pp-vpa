/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package trigger implements the dual-signal scale-up trigger (PSI +
// utilization) and the buffer-drop scale-down trigger.
package trigger

import "github.com/brycemclachlan/pp-vpa/internal/nodeagent/psi"

// ScaleUpDecision is the output of EvaluateScaleUp.
type ScaleUpDecision struct {
	Fire   bool
	Reason string
}

// EvaluateScaleUp fires when PSI.avg10 exceeds the configured threshold OR
// when utilization exceeds the request utilization threshold
// (requests * requestUtilThresholdPct / 100).
func EvaluateScaleUp(p psi.Line, psiThreshold float64, utilization, requests, requestUtilThresholdPct float64) ScaleUpDecision {
	if p.Avg10 >= psiThreshold && psiThreshold > 0 {
		return ScaleUpDecision{Fire: true, Reason: "psi"}
	}
	if requests > 0 && utilization >= requests*requestUtilThresholdPct/100.0 && requestUtilThresholdPct > 0 {
		return ScaleUpDecision{Fire: true, Reason: "utilization"}
	}
	return ScaleUpDecision{}
}
