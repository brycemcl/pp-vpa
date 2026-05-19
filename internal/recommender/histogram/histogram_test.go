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

package histogram

import (
	"math"
	"testing"
	"time"
)

func TestPercentileMonotonic(t *testing.T) {
	h, err := New(DefaultCPUOptions())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	for i := range 1000 {
		h.AddSample(float64(i%10)+0.1, 1.0, base)
	}
	p50 := h.Percentile(50)
	p90 := h.Percentile(90)
	p99 := h.Percentile(99)
	if !(p50 <= p90 && p90 <= p99) {
		t.Fatalf("percentiles not monotonic: %v %v %v", p50, p90, p99)
	}
}

func TestDecayHalvesWeight(t *testing.T) {
	opts := Options{
		MaxValue: 1000, FirstBucketSize: 1, BucketRatio: 1.1,
		HalfLife: time.Hour, Epsilon: 1e-9,
	}
	h, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(0, 0)
	h.AddSample(5.0, 1.0, t0)
	before := h.TotalWeight()
	h.Decay(t0.Add(time.Hour))
	after := h.TotalWeight()
	if math.Abs(after-before*0.5) > 1e-9 {
		t.Fatalf("expected weight to halve after one half-life: %v → %v", before, after)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	h, err := New(DefaultMemoryOptions())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(1700000000, 0)
	h.AddSample(1<<28, 1.0, t0)
	enc, err := Encode(h)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(h.TotalWeight()-h2.TotalWeight()) > 1e-9 {
		t.Fatalf("total weight diverged after round-trip: %v vs %v", h.TotalWeight(), h2.TotalWeight())
	}
}

func TestBucketIndexMonotonic(t *testing.T) {
	h, err := New(DefaultCPUOptions())
	if err != nil {
		t.Fatal(err)
	}
	prev := -1
	for v := 0.01; v < 100; v *= 1.5 {
		idx := h.BucketIndex(v)
		if idx < prev {
			t.Fatalf("BucketIndex non-monotonic at v=%v", v)
		}
		prev = idx
	}
}
