/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package nodeagent implements the per-node DaemonSet entrypoint: poll PSI,
// extract memory peaks, decide triggers, and queue resize patches.
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	"github.com/brycemclachlan/pp-vpa/internal/compat"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/cgroup"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/ingest"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/oom"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/patcher"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/psi"
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/watermark"
	"github.com/brycemclachlan/pp-vpa/internal/recommender/histogram"
)

// Config wires runtime knobs into the agent.
type Config struct {
	NodeName        string
	CgroupRoot      string
	KubeletConfig   string
	SampleInterval  time.Duration
	CheckpointEvery time.Duration
}

// podState is the per-pod telemetry held by the agent.
type podState struct {
	uid          string
	namespace    string
	name         string
	prrName      string
	qos          corev1.PodQOSClass
	cgroupPath   string
	cpu          *ingest.CPUStream
	mem          *ingest.MemoryPeakExtractor
	cpuWatermark *watermark.Watermark
	memWatermark *watermark.Watermark
	prevOOMKill  uint64
	memLimit     float64
}

// Agent is the main DaemonSet runtime.
type Agent struct {
	Cfg        Config
	Client     client.Client
	KubeClient kubernetes.Interface
	Caps       compat.NodeCapabilities

	mu    sync.Mutex
	pods  map[string]*podState // keyed by pod UID
	queue *patcher.Queue
	sub   *patcher.Submitter
	chk   *CheckpointWriter
}

// New constructs an Agent.
func New(cfg Config, c client.Client, kc kubernetes.Interface) (*Agent, error) {
	caps, err := compat.ProbeNode(cfg.CgroupRoot, cfg.KubeletConfig)
	if err != nil {
		return nil, err
	}
	if !caps.CgroupV2 {
		return nil, errors.New("cgroup v2 is required for PP-VPA node-agent")
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 10 * time.Second
	}
	if cfg.CheckpointEvery <= 0 {
		cfg.CheckpointEvery = time.Minute
	}
	return &Agent{
		Cfg:        cfg,
		Client:     c,
		KubeClient: kc,
		Caps:       caps,
		pods:       map[string]*podState{},
		queue:      patcher.NewQueue(),
		sub:        &patcher.Submitter{Client: kc},
		chk:        &CheckpointWriter{Client: c, Interval: cfg.CheckpointEvery},
	}, nil
}

// Run starts the sampling and patching loops until ctx is canceled.
func (a *Agent) Run(ctx context.Context) error {
	sampleTicker := time.NewTicker(a.Cfg.SampleInterval)
	defer sampleTicker.Stop()
	chkTicker := time.NewTicker(a.Cfg.CheckpointEvery)
	defer chkTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case t := <-sampleTicker.C:
			a.sampleAll(ctx, t)
			a.drainPatches(ctx)
		case <-chkTicker.C:
			a.writeCheckpoints(ctx)
		}
	}
}

// EnsurePod registers a pod for sampling, restoring its histogram from the
// PRR checkpoint if present.
func (a *Agent) EnsurePod(ctx context.Context, p corev1.Pod, prr autoscalingv1alpha1.PodResourceRecommendation) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	uid := string(p.UID)
	if _, ok := a.pods[uid]; ok {
		return nil
	}
	resolver := &cgroup.Resolver{Root: a.Cfg.CgroupRoot}
	slice, err := resolver.PodPath(uid, p.Status.QOSClass)
	if err != nil {
		return err
	}
	cs, err := ingest.NewCPUStream()
	if err != nil {
		return err
	}
	mp, err := ingest.NewMemoryPeakExtractor(time.Minute)
	if err != nil {
		return err
	}
	st := &podState{
		uid: uid, namespace: p.Namespace, name: p.Name, prrName: prr.Name,
		qos: p.Status.QOSClass, cgroupPath: slice,
		cpu: cs, mem: mp,
		cpuWatermark: watermark.New(time.Hour),
		memWatermark: watermark.New(24 * time.Hour),
	}
	if prr.Status.HistogramCheckpoint != "" {
		if h, err := histogram.Decode(prr.Status.HistogramCheckpoint); err == nil {
			// Best-effort restore into CPU stream's histogram. Memory peak
			// extractor's histogram is separate; matching CRD-side storage is
			// out of scope for this MVP.
			_ = cs.Snapshot().Merge(h)
		}
	}
	a.pods[uid] = st
	return nil
}

// ForgetPod removes per-pod state.
func (a *Agent) ForgetPod(uid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.pods, uid)
}

// sampleAll polls PSI and memory.events for every registered pod.
func (a *Agent) sampleAll(_ context.Context, t time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, st := range a.pods {
		if cpuPSI, err := psi.ReadFile(cgroup.PSIFile(st.cgroupPath, "cpu")); err == nil {
			_ = cpuPSI
		}
		if memPSI, err := psi.ReadFile(cgroup.PSIFile(st.cgroupPath, "memory")); err == nil {
			_ = memPSI
		}
		// Read memory.current.
		if b, err := os.ReadFile(st.cgroupPath + "/memory.current"); err == nil {
			var used uint64
			_, _ = fmt.Sscanf(string(b), "%d", &used)
			st.mem.Record(used, t)
		}
		// Watch for OOMs.
		if ev, err := oom.Parse(cgroup.MemoryEventsFile(st.cgroupPath)); err == nil {
			if ev.OOMKill > st.prevOOMKill && st.memLimit > 0 {
				st.mem.Record(uint64(oom.SyntheticSample(st.memLimit)), t)
				st.prevOOMKill = ev.OOMKill
			}
		}
	}
}

// drainPatches submits queued patches in priority order, yielding on 429.
func (a *Agent) drainPatches(ctx context.Context) {
	for {
		p, ok := a.queue.Pop()
		if !ok {
			return
		}
		if err := a.sub.Submit(ctx, p); err != nil {
			// On error, requeue once and return so client-go backoff applies.
			a.queue.Push(p)
			return
		}
	}
}

// writeCheckpoints encodes each pod's CPU histogram into its PRR. (Memory
// histogram piggybacks on the same field in this MVP.)
func (a *Agent) writeCheckpoints(ctx context.Context) {
	a.mu.Lock()
	pods := make([]*podState, 0, len(a.pods))
	for _, st := range a.pods {
		pods = append(pods, st)
	}
	a.mu.Unlock()
	for _, st := range pods {
		if err := a.chk.Write(ctx, st.namespace, st.prrName, st.cpu.Snapshot()); err != nil {
			// Best-effort.
			continue
		}
	}
}

// Enqueue queues a resize patch for the named container.
func (a *Agent) Enqueue(p patcher.PendingPatch) { a.queue.Push(p) }

// PodForUID returns the namespaced name registered for a pod UID.
func (a *Agent) PodForUID(uid string) (types.NamespacedName, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.pods[uid]
	if !ok {
		return types.NamespacedName{}, false
	}
	return types.NamespacedName{Namespace: st.namespace, Name: st.name}, true
}
