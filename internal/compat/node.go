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

// Package compat probes node and cluster capabilities to decide between
// pod-level and container-level-only operation.
package compat

import (
	"errors"
	"io"
	"os"
	"strings"
)

// NodeCapabilities describes what an individual node supports.
type NodeCapabilities struct {
	CgroupV2            bool
	SwapAccounting      bool
	StaticCPUManager    bool
	StaticMemoryManager bool
}

// ProbeNode inspects the kubelet config (default /var/lib/kubelet/config.yaml)
// and /sys/fs/cgroup to determine capabilities.
func ProbeNode(cgroupRoot, kubeletConfigPath string) (NodeCapabilities, error) {
	if cgroupRoot == "" {
		cgroupRoot = "/sys/fs/cgroup"
	}
	caps := NodeCapabilities{}
	if _, err := os.Stat(cgroupRoot + "/cgroup.controllers"); err == nil {
		caps.CgroupV2 = true
	}
	if _, err := os.Stat(cgroupRoot + "/memory.swap.events"); err == nil {
		caps.SwapAccounting = true
	}
	if kubeletConfigPath == "" {
		kubeletConfigPath = "/var/lib/kubelet/config.yaml"
	}
	f, err := os.Open(kubeletConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return caps, nil
		}
		return caps, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		return caps, err
	}
	if strings.Contains(string(b), "cpuManagerPolicy: static") {
		caps.StaticCPUManager = true
	}
	if strings.Contains(string(b), "memoryManagerPolicy: static") {
		caps.StaticMemoryManager = true
	}
	return caps, nil
}
