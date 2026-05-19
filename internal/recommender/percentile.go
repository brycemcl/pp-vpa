/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package recommender

import (
	"strconv"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

// Bounds holds the (lower, target, uncapped, upper) percentile values per resource.
type Bounds struct {
	CPULower, CPUTarget, CPUUncapped, CPUUpper             float64
	MemoryLower, MemoryTarget, MemoryUncapped, MemoryUpper float64
}

// ComputeBounds reads percentiles off the workload histograms using the
// PP-VPA's RecommenderPolicy. Strings ("90.0") are parsed to float; on
// parse failure, sensible defaults are used.
func ComputeBounds(wh *WorkloadHistograms, p autoscalingv1alpha1.RecommenderPolicy) Bounds {
	lower := parsePercentile(p.LowerBoundPercentile, 50.0)
	target := parsePercentile(p.TargetPercentile, 90.0)
	upper := parsePercentile(p.UpperBoundPercentile, 95.0)

	return Bounds{
		CPULower:       wh.CPU.Percentile(lower),
		CPUTarget:      wh.CPU.Percentile(target),
		CPUUncapped:    wh.CPU.Percentile(target),
		CPUUpper:       wh.CPU.Percentile(upper),
		MemoryLower:    wh.Memory.Percentile(lower),
		MemoryTarget:   wh.Memory.Percentile(target),
		MemoryUncapped: wh.Memory.Percentile(target),
		MemoryUpper:    wh.Memory.Percentile(upper),
	}
}

func parsePercentile(s string, fallback float64) float64 {
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return v
}
