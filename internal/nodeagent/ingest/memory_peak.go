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

package ingest

import (
	"sync"
	"time"

	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

// MemoryPeakExtractor stores only the absolute highest memory peak per
// interval; on Flush it emits one sample into the histogram and resets.
type MemoryPeakExtractor struct {
	mu       sync.Mutex
	h        *histogram.Histogram
	interval time.Duration
	curStart time.Time
	curPeak  uint64
	absPeak  uint64
}

// NewMemoryPeakExtractor returns an extractor with default histogram options.
func NewMemoryPeakExtractor(interval time.Duration) (*MemoryPeakExtractor, error) {
	h, err := histogram.New(histogram.DefaultMemoryOptions())
	if err != nil {
		return nil, err
	}
	return &MemoryPeakExtractor{h: h, interval: interval}, nil
}

// Record updates the current-interval peak with bytesUsed at time t.
func (m *MemoryPeakExtractor) Record(bytesUsed uint64, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.curStart.IsZero() {
		m.curStart = t
	}
	if bytesUsed > m.curPeak {
		m.curPeak = bytesUsed
	}
	if bytesUsed > m.absPeak {
		m.absPeak = bytesUsed
	}
	if t.Sub(m.curStart) >= m.interval {
		m.flushLocked(t)
	}
}

// Flush forces an end-of-interval flush at t.
func (m *MemoryPeakExtractor) Flush(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushLocked(t)
}

func (m *MemoryPeakExtractor) flushLocked(t time.Time) {
	if m.curPeak > 0 {
		m.h.AddSample(float64(m.curPeak), 1.0, t)
	}
	m.curPeak = 0
	m.curStart = t
}

// AbsolutePeak returns the highest sample seen since construction.
func (m *MemoryPeakExtractor) AbsolutePeak() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.absPeak
}

// Snapshot returns the underlying histogram. Callers must not mutate it.
func (m *MemoryPeakExtractor) Snapshot() *histogram.Histogram {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.h
}
