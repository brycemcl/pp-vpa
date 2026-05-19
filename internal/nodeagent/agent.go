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
	"k8s.io/apimachinery/pkg/api/resource"
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
	"github.com/brycemclachlan/pp-vpa/internal/nodeagent/validate"
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

// containerMeta holds per-container metadata needed for resize validation.
type containerMeta struct {
	containerType  validate.ContainerType
	currentCPU     resource.Quantity
	currentMemory  resource.Quantity
	resizePolicies []corev1.ContainerResizePolicy
}

// podState is the per-pod telemetry held by the agent.
type podState struct {
	uid          string
	namespace    string
	name         string
	prrName      string
	qos          corev1.PodQOSClass
	cgroupPath   string
	isWindows    bool
	containers   map[string]containerMeta
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

	isWindows := p.Spec.OS != nil && p.Spec.OS.Name == corev1.Windows
	containers := extractContainerMeta(&p)

	st := &podState{
		uid: uid, namespace: p.Namespace, name: p.Name, prrName: prr.Name,
		qos: p.Status.QOSClass, cgroupPath: slice,
		isWindows:  isWindows,
		containers: containers,
		cpu: cs, mem: mp,
		cpuWatermark: watermark.New(time.Hour),
		memWatermark: watermark.New(24 * time.Hour),
	}
	if prr.Status.HistogramCheckpoint != "" {
		if h, err := histogram.Decode(prr.Status.HistogramCheckpoint); err == nil {
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

// Enqueue validates and queues a resize patch for the named container.
// Returns an error if the resize violates Kubernetes 1.36 in-place resize
// limitations; in that case the patch is not queued.
func (a *Agent) Enqueue(p patcher.PendingPatch) error {
	a.mu.Lock()
	var st *podState
	for _, s := range a.pods {
		if s.name == p.Pod && s.namespace == p.Namespace {
			st = s
			break
		}
	}
	a.mu.Unlock()

	if st == nil {
		return fmt.Errorf("pod %s/%s not found in agent state", p.Namespace, p.Pod)
	}

	cm, ok := st.containers[p.Container]
	if !ok {
		return fmt.Errorf("container %q not found in pod %s/%s", p.Container, p.Namespace, p.Pod)
	}

	violations := validate.ValidateResize(validate.ResizeContext{
		NodeCaps:       a.Caps,
		QoS:            st.qos,
		IsWindows:      st.isWindows,
		ContainerType:  cm.containerType,
		ContainerName:  p.Container,
		CurrentCPU:     cm.currentCPU,
		CurrentMemory:  cm.currentMemory,
		ProposedCPU:    p.NewCPU,
		ProposedMemory: p.NewMemory,
		ResizePolicies: cm.resizePolicies,
	})
	if len(violations) > 0 {
		return fmt.Errorf("resize validation failed for %s/%s:%s: %v", p.Namespace, p.Pod, p.Container, violations)
	}

	a.queue.Push(p)
	return nil
}

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

// extractContainerMeta builds per-container metadata from a Pod object,
// classifying each container and recording its allocated resources and
// resize policies.
func extractContainerMeta(pod *corev1.Pod) map[string]containerMeta {
	m := make(map[string]containerMeta)

	// Build a lookup of allocated resources from container statuses.
	allocated := make(map[string]corev1.ResourceList)
	for i := range pod.Status.ContainerStatuses {
		cs := &pod.Status.ContainerStatuses[i]
		if cs.AllocatedResources != nil {
			allocated[cs.Name] = cs.AllocatedResources
		}
	}
	for i := range pod.Status.InitContainerStatuses {
		cs := &pod.Status.InitContainerStatuses[i]
		if cs.AllocatedResources != nil {
			allocated[cs.Name] = cs.AllocatedResources
		}
	}

	// Regular containers.
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		cm := containerMeta{containerType: validate.Regular}
		if ar, ok := allocated[c.Name]; ok {
			cm.currentCPU = ar[corev1.ResourceCPU]
			cm.currentMemory = ar[corev1.ResourceMemory]
		}
		cm.resizePolicies = c.ResizePolicy
		m[c.Name] = cm
	}

	// Init containers — classify as SidecarInit or NonRestartableInit.
	for i := range pod.Spec.InitContainers {
		c := &pod.Spec.InitContainers[i]
		cm := containerMeta{}
		if c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			cm.containerType = validate.SidecarInit
		} else {
			cm.containerType = validate.NonRestartableInit
		}
		if ar, ok := allocated[c.Name]; ok {
			cm.currentCPU = ar[corev1.ResourceCPU]
			cm.currentMemory = ar[corev1.ResourceMemory]
		}
		cm.resizePolicies = c.ResizePolicy
		m[c.Name] = cm
	}

	// Ephemeral containers.
	for i := range pod.Spec.EphemeralContainers {
		c := &pod.Spec.EphemeralContainers[i]
		cm := containerMeta{containerType: validate.Ephemeral}
		if ar, ok := allocated[c.Name]; ok {
			cm.currentCPU = ar[corev1.ResourceCPU]
			cm.currentMemory = ar[corev1.ResourceMemory]
		}
		m[c.Name] = cm
	}

	return m
}
