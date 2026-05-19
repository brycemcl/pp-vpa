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

package patcher

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/brycemclachlan/pp-vpa/internal/compat"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/validate"
)

func TestQueueDrainsHighestPriorityFirst(t *testing.T) {
	q := NewQueue()
	q.Push(PendingPatch{NodeTotalCPU: 10, NodeTotalMemory: 1 << 30, DeltaCPU: 1, DeltaMemory: 1 << 28, Pod: "small"})
	q.Push(PendingPatch{NodeTotalCPU: 10, NodeTotalMemory: 1 << 30, DeltaCPU: 5, DeltaMemory: 1 << 30, Pod: "big"})
	first, _ := q.Pop()
	if first.Pod != "big" {
		t.Fatalf("expected big patch first, got %q", first.Pod)
	}
}

func TestSubmitRejectsInvalidResize(t *testing.T) {
	s := &Submitter{NodeCaps: compat.NodeCapabilities{}}

	// BestEffort pod — should be rejected by validation.
	p := PendingPatch{
		Namespace:     "default",
		Pod:           "test-pod",
		Container:     "app",
		QoS:           corev1.PodQOSBestEffort,
		ContainerType: validate.Regular,
		CurrentCPU:    resource.MustParse("100m"),
		CurrentMemory: resource.MustParse("128Mi"),
		NewCPU:        resource.MustParse("200m"),
		NewMemory:     resource.MustParse("256Mi"),
	}
	err := s.Submit(nil, p)
	if err == nil {
		t.Fatal("expected validation error for BestEffort pod, got nil")
	}
}

func TestSubmitRejectsStaticCPUManager(t *testing.T) {
	s := &Submitter{NodeCaps: compat.NodeCapabilities{StaticCPUManager: true}}

	p := PendingPatch{
		Namespace:     "default",
		Pod:           "test-pod",
		Container:     "app",
		QoS:           corev1.PodQOSBurstable,
		ContainerType: validate.Regular,
		CurrentCPU:    resource.MustParse("100m"),
		CurrentMemory: resource.MustParse("128Mi"),
		NewCPU:        resource.MustParse("200m"),
		NewMemory:     resource.MustParse("256Mi"),
	}
	err := s.Submit(nil, p)
	if err == nil {
		t.Fatal("expected validation error for static CPU manager, got nil")
	}
}

func TestSubmitRejectsWindowsPod(t *testing.T) {
	// Submitter with no special node caps — but we test by setting
	// container type to Ephemeral which should be blocked.
	s := &Submitter{NodeCaps: compat.NodeCapabilities{}}

	p := PendingPatch{
		Namespace:     "default",
		Pod:           "test-pod",
		Container:     "debug",
		QoS:           corev1.PodQOSBurstable,
		ContainerType: validate.Ephemeral,
		CurrentCPU:    resource.MustParse("100m"),
		CurrentMemory: resource.MustParse("128Mi"),
		NewCPU:        resource.MustParse("200m"),
		NewMemory:     resource.MustParse("256Mi"),
	}
	err := s.Submit(nil, p)
	if err == nil {
		t.Fatal("expected validation error for ephemeral container, got nil")
	}
}

func TestPendingPatchPreservesValidationFields(t *testing.T) {
	q := NewQueue()
	p := PendingPatch{
		Namespace:       "ns",
		Pod:             "pod",
		Container:       "c",
		QoS:             corev1.PodQOSBurstable,
		ContainerType:   validate.SidecarInit,
		CurrentCPU:      resource.MustParse("50m"),
		CurrentMemory:   resource.MustParse("64Mi"),
		NewCPU:          resource.MustParse("100m"),
		NewMemory:       resource.MustParse("128Mi"),
		ResizePolicies:  []corev1.ContainerResizePolicy{{ResourceName: corev1.ResourceCPU, RestartPolicy: corev1.NotRequired}},
		NodeTotalCPU:    10,
		NodeTotalMemory: 1 << 30,
		DeltaCPU:        0.05,
		DeltaMemory:     64 << 20,
	}
	q.Push(p)
	got, ok := q.Pop()
	if !ok {
		t.Fatal("queue empty")
	}
	if got.QoS != corev1.PodQOSBurstable {
		t.Errorf("QoS = %q, want Burstable", got.QoS)
	}
	if got.ContainerType != validate.SidecarInit {
		t.Errorf("ContainerType = %d, want SidecarInit", got.ContainerType)
	}
	if got.CurrentCPU.Cmp(resource.MustParse("50m")) != 0 {
		t.Errorf("CurrentCPU = %s, want 50m", got.CurrentCPU.String())
	}
	if len(got.ResizePolicies) != 1 {
		t.Errorf("ResizePolicies len = %d, want 1", len(got.ResizePolicies))
	}
}
