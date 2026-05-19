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

package recommender

import (
	"testing"
	"time"

	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

func TestWorkloadCheckpointRoundTrip(t *testing.T) {
	cpu, _ := histogram.New(histogram.DefaultCPUOptions())
	cpu.AddSample(2.0, 1.0, time.Now())
	mem, _ := histogram.New(histogram.DefaultMemoryOptions())
	mem.AddSample(1<<30, 1.0, time.Now())

	wh := &WorkloadHistograms{CPU: cpu, Memory: mem}
	now := time.Now()

	encoded, err := EncodeWorkload(wh, now)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeWorkload(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.CPU.TotalWeight() == 0 {
		t.Fatal("expected non-zero CPU weight after round-trip")
	}
	if decoded.Memory.TotalWeight() == 0 {
		t.Fatal("expected non-zero memory weight after round-trip")
	}

	// Percentile values should be preserved.
	cpuP50 := cpu.Percentile(50)
	decP50 := decoded.CPU.Percentile(50)
	if cpuP50 != decP50 {
		t.Fatalf("CPU p50 not preserved: original=%v, decoded=%v", cpuP50, decP50)
	}
}

func TestWorkloadCheckpointEmptyHistograms(t *testing.T) {
	cpu, _ := histogram.New(histogram.DefaultCPUOptions())
	mem, _ := histogram.New(histogram.DefaultMemoryOptions())
	wh := &WorkloadHistograms{CPU: cpu, Memory: mem}

	encoded, err := EncodeWorkload(wh, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeWorkload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.CPU.TotalWeight() != 0 {
		t.Fatalf("expected zero CPU weight, got %v", decoded.CPU.TotalWeight())
	}
	if decoded.Memory.TotalWeight() != 0 {
		t.Fatalf("expected zero memory weight, got %v", decoded.Memory.TotalWeight())
	}
}

func TestWorkloadCheckpointCorruptString(t *testing.T) {
	_, err := DecodeWorkload("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for corrupt checkpoint string")
	}
}

func TestWorkloadCheckpointEmptyString(t *testing.T) {
	_, err := DecodeWorkload("")
	if err == nil {
		t.Fatal("expected error for empty checkpoint string")
	}
}
