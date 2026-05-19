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
	"testing"
	"time"
)

func TestCPUStreamRecordAndSnapshot(t *testing.T) {
	s, err := NewCPUStream()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.Record(2.0, now)
	h := s.Snapshot()
	if h.TotalWeight() == 0 {
		t.Fatal("expected non-zero weight after recording")
	}
	if h.Percentile(50) == 0 {
		t.Fatal("expected non-zero percentile after recording")
	}
}

func TestCPUStreamMultipleRecords(t *testing.T) {
	s, err := NewCPUStream()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 10; i++ {
		s.Record(float64(i+1), now.Add(time.Duration(i)*time.Second))
	}
	h := s.Snapshot()
	if h.TotalWeight() < 9 {
		t.Fatalf("expected weight >= 9, got %v", h.TotalWeight())
	}
}

func TestCPUStreamConcurrentAccess(t *testing.T) {
	s, err := NewCPUStream()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Record(float64(i%10+1), time.Now())
			_ = s.Snapshot()
		}(i)
	}
	wg.Wait()
	h := s.Snapshot()
	if h.TotalWeight() == 0 {
		t.Fatal("expected non-zero weight after concurrent writes")
	}
}
