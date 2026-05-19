/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package oom watches cgroup v2 memory.events for OOM kills and emits
// "bump-up" memory samples (previous_limit * bumpFactor) into the per-pod
// histogram.
package oom

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// DefaultBumpFactor is the synthetic multiplier applied to the limit when a
// container OOMs. The recommender then treats it as if the workload had
// actually peaked at that level.
const DefaultBumpFactor = 1.20

// Events represents the parsed contents of memory.events.
type Events struct {
	Low     uint64
	High    uint64
	Max     uint64
	OOM     uint64
	OOMKill uint64
}

// Parse decodes the memory.events file at path.
func Parse(path string) (Events, error) {
	f, err := os.Open(path)
	if err != nil {
		return Events{}, err
	}
	defer func() { _ = f.Close() }()
	var e Events
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		n, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "low":
			e.Low = n
		case "high":
			e.High = n
		case "max":
			e.Max = n
		case "oom":
			e.OOM = n
		case "oom_kill":
			e.OOMKill = n
		}
	}
	return e, sc.Err()
}

// SyntheticSample returns the bump-up sample (in bytes) to inject into the
// histogram when oom_kill increments, given the prior memory limit in bytes.
func SyntheticSample(priorLimitBytes float64) float64 {
	return priorLimitBytes * DefaultBumpFactor
}
