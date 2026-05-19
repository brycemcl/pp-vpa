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

// Package histogram implements a decaying exponential histogram inspired
// by sigs.k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/util.
// The histogram allocates buckets along an exponential schedule and ages
// samples via a half-life so recent observations dominate.
package histogram

import (
	"errors"
	"math"
	"time"
)

// Options configures a Histogram.
type Options struct {
	// MaxValue is the largest sample expected (samples beyond saturate into the last bucket).
	MaxValue float64
	// FirstBucketSize sets the granularity of the smallest bucket.
	FirstBucketSize float64
	// BucketRatio is the geometric growth ratio between buckets (>1).
	BucketRatio float64
	// HalfLife controls exponential decay; a sample older than HalfLife counts half.
	HalfLife time.Duration
	// Epsilon clips negligible buckets when serializing.
	Epsilon float64
}

// Validate checks Options sanity.
func (o Options) Validate() error {
	if o.MaxValue <= 0 {
		return errors.New("MaxValue must be > 0")
	}
	if o.FirstBucketSize <= 0 {
		return errors.New("FirstBucketSize must be > 0")
	}
	if o.BucketRatio <= 1 {
		return errors.New("BucketRatio must be > 1")
	}
	if o.HalfLife <= 0 {
		return errors.New("HalfLife must be > 0")
	}
	if o.Epsilon < 0 {
		return errors.New("Epsilon must be >= 0")
	}
	return nil
}

// numBuckets returns the bucket count for the given Options.
// First bucket covers [0, FirstBucketSize). Subsequent buckets grow geometrically.
func numBuckets(o Options) int {
	// Solve FirstBucketSize * (ratio^n - 1) / (ratio - 1) >= MaxValue.
	r := o.BucketRatio
	cap := o.MaxValue * (r - 1) / o.FirstBucketSize
	n := int(math.Ceil(math.Log(cap+1) / math.Log(r)))
	if n < 1 {
		return 1
	}
	return n
}

// Histogram is a decaying weighted histogram.
type Histogram struct {
	opts        Options
	n           int
	buckets     []float64
	totalWeight float64
	// referenceTime is the timestamp the buckets are aged to.
	referenceTime time.Time
}

// New constructs a Histogram. Returns nil on invalid options.
func New(o Options) (*Histogram, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	n := numBuckets(o)
	return &Histogram{
		opts:    o,
		n:       n,
		buckets: make([]float64, n),
	}, nil
}

// BucketCount exposes the bucket count for tests.
func (h *Histogram) BucketCount() int { return h.n }

// Options returns a copy of the histogram's options.
func (h *Histogram) Options() Options { return h.opts }

// BucketIndex maps a sample to its bucket index.
func (h *Histogram) BucketIndex(value float64) int {
	if value <= 0 {
		return 0
	}
	// Solve FirstBucketSize * (ratio^k - 1) / (ratio - 1) >= value, return k.
	r := h.opts.BucketRatio
	idx := int(math.Floor(math.Log(value*(r-1)/h.opts.FirstBucketSize+1) / math.Log(r)))
	if idx < 0 {
		return 0
	}
	if idx >= h.n {
		return h.n - 1
	}
	return idx
}

// BucketStart returns the lower edge of bucket k.
func (h *Histogram) BucketStart(k int) float64 {
	if k <= 0 {
		return 0
	}
	r := h.opts.BucketRatio
	return h.opts.FirstBucketSize * (math.Pow(r, float64(k)) - 1) / (r - 1)
}

// AddSample records a value at sampleTime with a weight.
func (h *Histogram) AddSample(value, weight float64, sampleTime time.Time) {
	if weight <= 0 {
		return
	}
	h.shiftReference(sampleTime)
	idx := h.BucketIndex(value)
	h.buckets[idx] += weight
	h.totalWeight += weight
}

// shiftReference ages all buckets to t, applying exponential decay.
func (h *Histogram) shiftReference(t time.Time) {
	if h.referenceTime.IsZero() {
		h.referenceTime = t
		return
	}
	if !t.After(h.referenceTime) {
		return
	}
	dt := t.Sub(h.referenceTime).Seconds()
	half := h.opts.HalfLife.Seconds()
	if half <= 0 {
		return
	}
	factor := math.Pow(0.5, dt/half)
	for i := range h.buckets {
		h.buckets[i] *= factor
	}
	h.totalWeight *= factor
	h.referenceTime = t
}

// Decay ages buckets without inserting a sample.
func (h *Histogram) Decay(t time.Time) { h.shiftReference(t) }

// Percentile returns the value at the requested percentile (0-100).
// Returns 0 when the histogram is empty.
func (h *Histogram) Percentile(p float64) float64 {
	if p < 0 {
		p = 0
	} else if p > 100 {
		p = 100
	}
	if h.totalWeight <= 0 {
		return 0
	}
	target := h.totalWeight * (p / 100.0)
	var cum float64
	for i, w := range h.buckets {
		cum += w
		if cum >= target {
			// Return upper edge of the bucket for a stable, conservative estimate.
			if i == h.n-1 {
				return h.opts.MaxValue
			}
			return h.BucketStart(i + 1)
		}
	}
	return h.opts.MaxValue
}

// IsEmpty returns true if no weight has accumulated.
func (h *Histogram) IsEmpty() bool { return h.totalWeight <= 0 }

// Merge adds the contents of other into h. Both must share Options.
func (h *Histogram) Merge(other *Histogram) error {
	if h.opts != other.opts {
		return errors.New("incompatible histogram options")
	}
	// Re-base both to the later reference time.
	t := h.referenceTime
	if other.referenceTime.After(t) {
		t = other.referenceTime
	}
	h.shiftReference(t)
	cp := *other
	cp.buckets = make([]float64, len(other.buckets))
	copy(cp.buckets, other.buckets)
	cp.shiftReference(t)
	for i := range h.buckets {
		h.buckets[i] += cp.buckets[i]
	}
	h.totalWeight += cp.totalWeight
	return nil
}

// Buckets exposes internal state for serialization.
func (h *Histogram) Buckets() []float64 {
	out := make([]float64, len(h.buckets))
	copy(out, h.buckets)
	return out
}

// LoadBuckets replaces internal state from a serializer.
func (h *Histogram) LoadBuckets(b []float64, totalWeight float64, ref time.Time) error {
	if len(b) != h.n {
		return errors.New("bucket count mismatch")
	}
	copy(h.buckets, b)
	h.totalWeight = totalWeight
	h.referenceTime = ref
	return nil
}

// ReferenceTime returns the histogram's current decay reference.
func (h *Histogram) ReferenceTime() time.Time { return h.referenceTime }

// TotalWeight returns the sum of bucket weights.
func (h *Histogram) TotalWeight() float64 { return h.totalWeight }

// DefaultCPUOptions returns sensible defaults for CPU histograms (cores).
func DefaultCPUOptions() Options {
	return Options{
		MaxValue:        1000.0, // up to 1000 cores
		FirstBucketSize: 0.01,
		BucketRatio:     1.05,
		HalfLife:        24 * time.Hour,
		Epsilon:         1e-6,
	}
}

// DefaultMemoryOptions returns sensible defaults for memory histograms (bytes).
func DefaultMemoryOptions() Options {
	return Options{
		MaxValue:        1 << 40, // 1 TiB
		FirstBucketSize: 1 << 20, // 1 MiB
		BucketRatio:     1.05,
		HalfLife:        24 * time.Hour,
		Epsilon:         1e-6,
	}
}
