/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package psi

import (
	"strings"
	"testing"
)

func TestParseFullAndSome(t *testing.T) {
	in := `some avg10=1.23 avg60=4.56 avg300=7.89 total=12345
full avg10=0.11 avg60=0.22 avg300=0.33 total=67890`
	s, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if s.Some.Avg10 != 1.23 || s.Some.TotalUS != 12345 {
		t.Fatalf("some line wrong: %+v", s.Some)
	}
	if s.Full.Avg60 != 0.22 || s.Full.TotalUS != 67890 {
		t.Fatalf("full line wrong: %+v", s.Full)
	}
}

func TestParseSomeOnly(t *testing.T) {
	in := `some avg10=0.00 avg60=0.00 avg300=0.00 total=0`
	s, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if s.Full.TotalUS != 0 {
		t.Fatalf("full should be zero, got %+v", s.Full)
	}
}
