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
	"testing"
	"time"
)

func TestMemoryPeakRecordHigherKeepsValue(t *testing.T) {
	m, err := NewMemoryPeakExtractor(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.Record(100, now)
	m.Record(200, now)
	m.Record(50, now)
	m.Flush(now)
	h := m.Snapshot()
	if h.TotalWeight() == 0 {
		t.Fatal("expected non-zero weight after flush")
	}
	p := h.Percentile(100)
	if p < 200 {
		t.Fatalf("expected peak >= 200, got %v", p)
	}
}

func TestMemoryPeakLowerValueIgnored(t *testing.T) {
	m, err := NewMemoryPeakExtractor(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.Record(100, now)
	m.Record(50, now)
	m.Flush(now)
	h := m.Snapshot()
	p := h.Percentile(100)
	if p < 100 {
		t.Fatalf("expected peak >= 100 (lower value should not decrease), got %v", p)
	}
}

func TestMemoryPeakFlushEmitsAndResets(t *testing.T) {
	m, err := NewMemoryPeakExtractor(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.Record(300, now)
	m.Flush(now)
	// After flush, histogram should have one sample.
	h1 := m.Snapshot()
	if h1.TotalWeight() == 0 {
		t.Fatal("expected weight after flush")
	}

	// After flush, current peak resets. Record a new value and flush again.
	m.Record(100, now.Add(time.Second))
	m.Flush(now.Add(time.Second))
	h2 := m.Snapshot()
	// Two samples total means p100 should reflect the max of both.
	p := h2.Percentile(100)
	if p < 300 {
		t.Fatalf("expected p100 >= 300 (first peak), got %v", p)
	}
}

func TestMemoryPeakAllTimePeak(t *testing.T) {
	m, err := NewMemoryPeakExtractor(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.Record(100, now)
	m.Record(500, now)
	m.Flush(now)
	m.Record(200, now.Add(time.Minute))
	m.Flush(now.Add(time.Minute))

	abs := m.AbsolutePeak()
	if abs != 500 {
		t.Fatalf("expected all-time peak=500, got %v", abs)
	}
}

func TestMemoryPeakEmptyBeforeFlush(t *testing.T) {
	m, err := NewMemoryPeakExtractor(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Snapshot()
	if h.TotalWeight() != 0 {
		t.Fatalf("expected zero weight before any flush, got %v", h.TotalWeight())
	}
}

func TestMemoryPeakIntervalFlush(t *testing.T) {
	m, err := NewMemoryPeakExtractor(10 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.Record(400, now)
	// After interval passes, next Record should auto-flush.
	m.Record(100, now.Add(20*time.Millisecond))
	h := m.Snapshot()
	if h.TotalWeight() == 0 {
		t.Fatal("expected auto-flush after interval")
	}
}
