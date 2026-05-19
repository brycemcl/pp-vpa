/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package psi reads Linux Pressure Stall Information files (cpu.pressure,
// memory.pressure, io.pressure) using the cgroup v2 layout.
//
// File format (one or two lines):
//
//	some avg10=0.00 avg60=0.00 avg300=0.00 total=0
//	full avg10=0.00 avg60=0.00 avg300=0.00 total=0
package psi

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Line represents one row of a PSI file (either "some" or "full").
type Line struct {
	Avg10   float64
	Avg60   float64
	Avg300  float64
	TotalUS uint64
}

// Stats holds the parsed contents of a PSI file.
type Stats struct {
	Some Line
	Full Line // zero if the file omits the "full" line (CPU does on some kernels)
}

// ReadFile parses the PSI file at path.
func ReadFile(path string) (Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return Stats{}, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse parses PSI content from an io.Reader.
func Parse(r io.Reader) (Stats, error) {
	var s Stats
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return Stats{}, fmt.Errorf("malformed PSI line: %q", line)
		}
		l := Line{}
		for _, kv := range fields[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return Stats{}, fmt.Errorf("malformed PSI kv: %q", kv)
			}
			switch k {
			case "avg10":
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return Stats{}, err
				}
				l.Avg10 = f
			case "avg60":
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return Stats{}, err
				}
				l.Avg60 = f
			case "avg300":
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return Stats{}, err
				}
				l.Avg300 = f
			case "total":
				n, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return Stats{}, err
				}
				l.TotalUS = n
			}
		}
		switch fields[0] {
		case "some":
			s.Some = l
		case "full":
			s.Full = l
		}
	}
	return s, sc.Err()
}
