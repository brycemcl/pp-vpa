/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package patcher

import "testing"

func TestQueueDrainsHighestPriorityFirst(t *testing.T) {
	q := NewQueue()
	q.Push(PendingPatch{NodeTotalCPU: 10, NodeTotalMemory: 1 << 30, DeltaCPU: 1, DeltaMemory: 1 << 28, Pod: "small"})
	q.Push(PendingPatch{NodeTotalCPU: 10, NodeTotalMemory: 1 << 30, DeltaCPU: 5, DeltaMemory: 1 << 30, Pod: "big"})
	first, _ := q.Pop()
	if first.Pod != "big" {
		t.Fatalf("expected big patch first, got %q", first.Pod)
	}
}
