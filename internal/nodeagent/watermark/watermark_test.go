/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package watermark

import (
	"testing"
	"time"
)

func TestRecordHigherKeepsValue(t *testing.T) {
	w := New(24 * time.Hour)
	t0 := time.Unix(0, 0)
	w.Record(10, t0)
	w.Record(5, t0.Add(time.Minute))
	if got := w.Read(t0.Add(time.Minute)); got < 9.9 {
		t.Fatalf("watermark should hold near 10, got %v", got)
	}
}

func TestDecayToZero(t *testing.T) {
	w := New(time.Hour)
	t0 := time.Unix(0, 0)
	w.Record(100, t0)
	if got := w.Read(t0.Add(time.Hour)); got != 0 {
		t.Fatalf("expected full decay to 0, got %v", got)
	}
}

func TestPartialDecay(t *testing.T) {
	w := New(2 * time.Hour)
	t0 := time.Unix(0, 0)
	w.Record(100, t0)
	got := w.Read(t0.Add(time.Hour))
	if got < 49 || got > 51 {
		t.Fatalf("expected ~50 after half window, got %v", got)
	}
}
