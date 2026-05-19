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

// Package ingest feeds CPU and memory samples into per-pod decaying
// histograms. CPU is a continuous stream; memory uses interval-peak extraction.
package ingest

import (
	"sync"
	"time"

	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

// CPUStream is a continuous decaying CPU usage histogram.
type CPUStream struct {
	mu sync.Mutex
	h  *histogram.Histogram
}

// NewCPUStream allocates a CPUStream with default histogram options.
func NewCPUStream() (*CPUStream, error) {
	h, err := histogram.New(histogram.DefaultCPUOptions())
	if err != nil {
		return nil, err
	}
	return &CPUStream{h: h}, nil
}

// Record adds a CPU usage sample in cores at the given time.
func (s *CPUStream) Record(coresUsed float64, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h.AddSample(coresUsed, 1.0, t)
}

// Snapshot returns the underlying histogram. Callers must not mutate it.
func (s *CPUStream) Snapshot() *histogram.Histogram {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h
}
