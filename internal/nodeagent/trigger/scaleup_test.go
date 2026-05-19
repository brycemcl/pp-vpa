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

package trigger

import (
	"testing"

	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/psi"
)

func TestEvaluateScaleUpPSIReactive(t *testing.T) {
	p := psi.Line{Avg10: 15.0}
	d := EvaluateScaleUp(p, 10.0, 0, 0, 0)
	if !d.Fire || d.Reason != "psi" {
		t.Fatalf("expected PSI trigger fire, got fire=%v reason=%v", d.Fire, d.Reason)
	}
}

func TestEvaluateScaleUpPSIThresholdZero(t *testing.T) {
	p := psi.Line{Avg10: 99.0}
	d := EvaluateScaleUp(p, 0, 0, 0, 0)
	if d.Fire {
		t.Fatal("expected no fire when PSI threshold is 0")
	}
}

func TestEvaluateScaleUpUtilizationProactive(t *testing.T) {
	p := psi.Line{Avg10: 0}
	d := EvaluateScaleUp(p, 10.0, 900, 1000, 80)
	if !d.Fire || d.Reason != "utilization" {
		t.Fatalf("expected utilization trigger fire, got fire=%v reason=%v", d.Fire, d.Reason)
	}
}

func TestEvaluateScaleUpBothTriggers(t *testing.T) {
	p := psi.Line{Avg10: 20.0}
	d := EvaluateScaleUp(p, 10.0, 900, 1000, 80)
	if !d.Fire {
		t.Fatal("expected fire when both PSI and utilization conditions met")
	}
}

func TestEvaluateScaleUpNoTrigger(t *testing.T) {
	p := psi.Line{Avg10: 5.0}
	d := EvaluateScaleUp(p, 10.0, 50, 1000, 80)
	if d.Fire {
		t.Fatal("expected no fire when neither condition met")
	}
}

func TestEvaluateScaleUpZeroRequests(t *testing.T) {
	p := psi.Line{Avg10: 0}
	d := EvaluateScaleUp(p, 10.0, 900, 0, 80)
	if d.Fire {
		t.Fatal("expected no fire when requests=0 disables utilization trigger")
	}
}

func TestEvaluateScaleUpZeroUtilizationThreshold(t *testing.T) {
	p := psi.Line{Avg10: 0}
	d := EvaluateScaleUp(p, 10.0, 900, 1000, 0)
	if d.Fire {
		t.Fatal("expected no fire when utilization threshold=0")
	}
}
