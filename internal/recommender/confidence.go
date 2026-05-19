/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package recommender

import (
	"math"
	"time"
)

// Confidence widens the (lower, upper) interval when little history exists.
// The multipliers approach 1.0 as the workload accumulates more lifetime.
// This mirrors the formula in the upstream VPA recommender: factor =
// (1 + 1/(hours/24))^p.
type Confidence struct {
	LowerMultiplier float64
	UpperMultiplier float64
}

// ConfidenceMultipliers computes lower/upper widening factors from the
// lifetime of the workload.
func ConfidenceMultipliers(lifetime time.Duration) Confidence {
	hours := lifetime.Hours()
	if hours < 1.0/60.0 {
		hours = 1.0 / 60.0 // floor at 1 minute
	}
	t := hours / 24.0
	// Lower bound: widen downward (smaller values) with low history.
	// Upper bound: widen upward (larger values) with low history.
	lower := math.Pow(1.0+1.0/t, -0.5)
	upper := math.Pow(1.0+1.0/t, 1.0)
	return Confidence{LowerMultiplier: lower, UpperMultiplier: upper}
}

// ApplyConfidence widens bounds in-place using a multiplier derived from
// the workload's lifetime.
func ApplyConfidence(b Bounds, c Confidence) Bounds {
	b.CPULower *= c.LowerMultiplier
	b.CPUUpper *= c.UpperMultiplier
	b.MemoryLower *= c.LowerMultiplier
	b.MemoryUpper *= c.UpperMultiplier
	return b
}
