/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package oom

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "memory.events")
	if err := os.WriteFile(p, []byte("low 0\nhigh 0\nmax 0\noom 1\noom_kill 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if e.OOM != 1 || e.OOMKill != 2 {
		t.Fatalf("got %+v", e)
	}
}

func TestSyntheticBumpFactor(t *testing.T) {
	got := SyntheticSample(1024)
	if got != 1024*DefaultBumpFactor {
		t.Fatalf("bump factor not applied: got %v", got)
	}
}
