/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package watermark tracks per-PRR decaying high-watermarks for CPU and
// memory. A watermark drops linearly to zero over the configured decay window
// if no new peak hits it.
package watermark

import (
	"sync"
	"time"
)

// Watermark is a single decaying high-watermark.
type Watermark struct {
	mu        sync.Mutex
	value     float64
	updatedAt time.Time
	window    time.Duration
}

// New constructs a Watermark with the given decay window.
func New(window time.Duration) *Watermark {
	return &Watermark{window: window}
}

// Record bumps the watermark if v exceeds the decayed current value.
func (w *Watermark) Record(v float64, t time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cur := w.decayedLocked(t)
	if v > cur {
		w.value = v
		w.updatedAt = t
		return
	}
	// Persist the decayed value so successive Reads return a fresh number.
	w.value = cur
	w.updatedAt = t
}

// Read returns the decayed watermark value at time t.
func (w *Watermark) Read(t time.Time) float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.decayedLocked(t)
}

func (w *Watermark) decayedLocked(t time.Time) float64 {
	if w.window <= 0 || w.updatedAt.IsZero() {
		return w.value
	}
	elapsed := t.Sub(w.updatedAt)
	if elapsed <= 0 {
		return w.value
	}
	if elapsed >= w.window {
		return 0
	}
	frac := 1.0 - float64(elapsed)/float64(w.window)
	return w.value * frac
}
