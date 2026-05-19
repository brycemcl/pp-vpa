/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package recommender computes workload-wide recommendations from per-PRR
// histograms.
package recommender

import (
	"fmt"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

// WorkloadHistograms holds the per-resource aggregated histograms.
type WorkloadHistograms struct {
	CPU    *histogram.Histogram
	Memory *histogram.Histogram
}

// NewWorkloadHistograms allocates empty aggregate histograms.
func NewWorkloadHistograms() (*WorkloadHistograms, error) {
	cpu, err := histogram.New(histogram.DefaultCPUOptions())
	if err != nil {
		return nil, fmt.Errorf("cpu histogram: %w", err)
	}
	mem, err := histogram.New(histogram.DefaultMemoryOptions())
	if err != nil {
		return nil, fmt.Errorf("memory histogram: %w", err)
	}
	return &WorkloadHistograms{CPU: cpu, Memory: mem}, nil
}

// Aggregate merges every PRR's histogramCheckpoint into a workload-wide
// pair of decaying histograms. PRRs without a checkpoint are skipped.
func Aggregate(prrs []autoscalingv1alpha1.PodResourceRecommendation) (*WorkloadHistograms, error) {
	wh, err := NewWorkloadHistograms()
	if err != nil {
		return nil, err
	}
	for i := range prrs {
		ck := prrs[i].Status.HistogramCheckpoint
		if ck == "" {
			continue
		}
		h, err := histogram.Decode(ck)
		if err != nil {
			// A corrupt checkpoint should not poison the whole aggregate.
			continue
		}
		// Best-effort: merge into the matching aggregate by bucket-shape comparison.
		// We carry separate CPU and Memory checkpoints encoded per-resource elsewhere;
		// here we treat every PRR-encoded histogram as opaque and merge defensively.
		if h.Options().MaxValue >= 1<<30 {
			_ = wh.Memory.Merge(h)
		} else {
			_ = wh.CPU.Merge(h)
		}
	}
	return wh, nil
}
