/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package patcher builds PATCH requests against the kubelet's in-place
// pod resize subresource (KEP-1287). Pending patches are drained in priority
// order by |ΔCPU/NodeTotalCPU| + |ΔMemory/NodeTotalMem|; congestion control is
// delegated to API Priority and Fairness (see config/apf/).
package patcher

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// PendingPatch is one in-place resize request awaiting submission.
type PendingPatch struct {
	Namespace string
	Pod       string
	Container string
	NewCPU    resource.Quantity
	NewMemory resource.Quantity

	NodeTotalCPU    float64 // cores
	NodeTotalMemory float64 // bytes
	DeltaCPU        float64 // cores
	DeltaMemory     float64 // bytes
}

// Priority is the impact-magnitude score; larger = drained first.
func (p PendingPatch) Priority() float64 {
	score := 0.0
	if p.NodeTotalCPU > 0 {
		score += absf(p.DeltaCPU) / p.NodeTotalCPU
	}
	if p.NodeTotalMemory > 0 {
		score += absf(p.DeltaMemory) / p.NodeTotalMemory
	}
	return score
}

// Queue is a max-heap of PendingPatch by Priority.
type Queue struct {
	mu sync.Mutex
	h  patchHeap
}

// NewQueue returns an empty Queue.
func NewQueue() *Queue { return &Queue{} }

// Push enqueues p.
func (q *Queue) Push(p PendingPatch) {
	q.mu.Lock()
	defer q.mu.Unlock()
	heap.Push(&q.h, p)
}

// Pop returns the highest-priority pending patch and a bool indicating whether
// the queue had anything to return.
func (q *Queue) Pop() (PendingPatch, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() == 0 {
		return PendingPatch{}, false
	}
	return heap.Pop(&q.h).(PendingPatch), true
}

// Len returns the queue length.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.h.Len()
}

// patchHeap is a max-heap by Priority.
type patchHeap []PendingPatch

func (h patchHeap) Len() int           { return len(h) }
func (h patchHeap) Less(i, j int) bool { return h[i].Priority() > h[j].Priority() }
func (h patchHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *patchHeap) Push(x any)        { *h = append(*h, x.(PendingPatch)) }
func (h *patchHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Submitter wraps the kubelet's pods/<name>/resize subresource.
type Submitter struct {
	Client kubernetes.Interface
}

// Submit PATCHes the pod's resize subresource with the new resources for the
// named container. Returns a kubernetes apierror so callers can dispatch on
// 429 (Retry-After / APF) vs Forbidden (KEP-1287 unsupported).
func (s *Submitter) Submit(ctx context.Context, p PendingPatch) error {
	type containerPatch struct {
		Name      string                      `json:"name"`
		Resources corev1.ResourceRequirements `json:"resources"`
	}
	type podSpec struct {
		Containers []containerPatch `json:"containers"`
	}
	type podPatch struct {
		Spec podSpec `json:"spec"`
	}
	body, err := json.Marshal(podPatch{Spec: podSpec{Containers: []containerPatch{{
		Name: p.Container,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    p.NewCPU,
				corev1.ResourceMemory: p.NewMemory,
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    p.NewCPU,
				corev1.ResourceMemory: p.NewMemory,
			},
		},
	}}}})
	if err != nil {
		return err
	}
	_, err = s.Client.CoreV1().Pods(p.Namespace).Patch(
		ctx,
		p.Pod,
		types.StrategicMergePatchType,
		body,
		patchOpts(),
		"resize",
	)
	if err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsInvalid(err) {
			return fmt.Errorf("resize rejected by kubelet (likely KEP-1287 unsupported): %w", err)
		}
		return err
	}
	return nil
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
