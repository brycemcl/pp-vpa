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

package compat

import (
	"os"
	"path/filepath"
	"testing"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var apfScheme = runtime.NewScheme()

func init() {
	_ = flowcontrolv1.AddToScheme(apfScheme)
}

func decodeAPF(t *testing.T, filename string) runtime.Object {
	t.Helper()
	projDir := os.Getenv("PPVPA_ROOT")
	if projDir == "" {
		projDir = filepath.Join("..", "..")
	}
	data, err := os.ReadFile(filepath.Join(projDir, "config", "apf", filename))
	if err != nil {
		t.Skipf("APF file not found: %v", err)
	}
	decode := serializer.NewCodecFactory(apfScheme).UniversalDeserializer()
	obj, _, err := decode.Decode(data, nil, nil)
	if err != nil {
		t.Fatalf("failed to decode %s: %v", filename, err)
	}
	return obj
}

func TestAPFControlPlaneFlowSchema(t *testing.T) {
	obj := decodeAPF(t, "flowschema-pp-vpa-control-plane.yaml")
	fs, ok := obj.(*flowcontrolv1.FlowSchema)
	if !ok {
		t.Fatal("expected FlowSchema")
	}
	if fs.Spec.MatchingPrecedence != 800 {
		t.Fatalf("expected matchingPrecedence=800, got %d", fs.Spec.MatchingPrecedence)
	}
	if fs.Spec.DistinguisherMethod == nil || fs.Spec.DistinguisherMethod.Type != flowcontrolv1.FlowDistinguisherMethodByUserType {
		t.Fatal("expected distinguisherMethod.type=ByUser")
	}
	if fs.Spec.PriorityLevelConfiguration.Name != "pp-vpa-control-plane" {
		t.Fatalf("expected priorityLevelConfiguration.name=pp-vpa-control-plane, got %s", fs.Spec.PriorityLevelConfiguration.Name)
	}
}

func TestAPFControlPlanePriorityLevel(t *testing.T) {
	obj := decodeAPF(t, "prioritylevel-pp-vpa-control-plane.yaml")
	pl, ok := obj.(*flowcontrolv1.PriorityLevelConfiguration)
	if !ok {
		t.Fatal("expected PriorityLevelConfiguration")
	}
	if pl.Spec.Type != flowcontrolv1.PriorityLevelEnablementLimited {
		t.Fatalf("expected type=Limited, got %s", pl.Spec.Type)
	}
	if pl.Spec.Limited == nil {
		t.Fatal("expected limited config")
	}
	if pl.Spec.Limited.NominalConcurrencyShares == nil || *pl.Spec.Limited.NominalConcurrencyShares != 30 {
		t.Fatalf("expected nominalConcurrencyShares=30, got %v", pl.Spec.Limited.NominalConcurrencyShares)
	}
	q := pl.Spec.Limited.LimitResponse.Queuing
	if q == nil {
		t.Fatal("expected queuing config")
	}
	if q.QueueLengthLimit != 100 {
		t.Fatalf("expected queueLengthLimit=100, got %d", q.QueueLengthLimit)
	}
	if q.Queues != 32 {
		t.Fatalf("expected queues=32, got %d", q.Queues)
	}
	if q.HandSize != 6 {
		t.Fatalf("expected handSize=6, got %d", q.HandSize)
	}
}

func TestAPFNodeAgentsFlowSchema(t *testing.T) {
	obj := decodeAPF(t, "flowschema-pp-vpa-node-agents.yaml")
	fs, ok := obj.(*flowcontrolv1.FlowSchema)
	if !ok {
		t.Fatal("expected FlowSchema")
	}
	if fs.Spec.MatchingPrecedence != 850 {
		t.Fatalf("expected matchingPrecedence=850, got %d", fs.Spec.MatchingPrecedence)
	}
	if fs.Spec.DistinguisherMethod == nil || fs.Spec.DistinguisherMethod.Type != flowcontrolv1.FlowDistinguisherMethodByUserType {
		t.Fatal("expected distinguisherMethod.type=ByUser")
	}
	if fs.Spec.PriorityLevelConfiguration.Name != "pp-vpa-node-agents" {
		t.Fatalf("expected priorityLevelConfiguration.name=pp-vpa-node-agents, got %s", fs.Spec.PriorityLevelConfiguration.Name)
	}
}

func TestAPFNodeAgentsPriorityLevel(t *testing.T) {
	obj := decodeAPF(t, "prioritylevel-pp-vpa-node-agents.yaml")
	pl, ok := obj.(*flowcontrolv1.PriorityLevelConfiguration)
	if !ok {
		t.Fatal("expected PriorityLevelConfiguration")
	}
	if pl.Spec.Type != flowcontrolv1.PriorityLevelEnablementLimited {
		t.Fatalf("expected type=Limited, got %s", pl.Spec.Type)
	}
	if pl.Spec.Limited == nil {
		t.Fatal("expected limited config")
	}
	if pl.Spec.Limited.NominalConcurrencyShares == nil || *pl.Spec.Limited.NominalConcurrencyShares != 50 {
		t.Fatalf("expected nominalConcurrencyShares=50, got %v", pl.Spec.Limited.NominalConcurrencyShares)
	}
	q := pl.Spec.Limited.LimitResponse.Queuing
	if q == nil {
		t.Fatal("expected queuing config")
	}
	if q.QueueLengthLimit != 200 {
		t.Fatalf("expected queueLengthLimit=200, got %d", q.QueueLengthLimit)
	}
	if q.Queues != 128 {
		t.Fatalf("expected queues=128, got %d", q.Queues)
	}
	if q.HandSize != 8 {
		t.Fatalf("expected handSize=8, got %d", q.HandSize)
	}
}
