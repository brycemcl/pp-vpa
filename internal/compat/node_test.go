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
)

func TestProbeNodeCgroupV2(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0755)
	f, _ := os.Create(filepath.Join(dir, "cgroup.controllers"))
	f.Close()

	c, err := ProbeNode(dir, filepath.Join(t.TempDir(), "kubelet-config"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.CgroupV2 {
		t.Fatal("expected CgroupV2=true")
	}
}

func TestProbeNodeCgroupV1(t *testing.T) {
	dir := t.TempDir()
	c, err := ProbeNode(dir, filepath.Join(t.TempDir(), "kubelet-config"))
	if err != nil {
		t.Fatal(err)
	}
	if c.CgroupV2 {
		t.Fatal("expected CgroupV2=false for v1")
	}
}

func TestProbeNodeSwapAccounting(t *testing.T) {
	dir := t.TempDir()
	f, _ := os.Create(filepath.Join(dir, "memory.swap.events"))
	f.Close()

	c, err := ProbeNode(dir, filepath.Join(t.TempDir(), "kubelet-config"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.SwapAccounting {
		t.Fatal("expected SwapAccounting=true")
	}
}

func TestProbeNodeStaticCPUManager(t *testing.T) {
	kubeDir := t.TempDir()
	kubeletCfg := filepath.Join(kubeDir, "kubelet-config.yaml")
	os.WriteFile(kubeletCfg, []byte("cpuManagerPolicy: static\n"), 0644)

	c, err := ProbeNode(t.TempDir(), kubeletCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !c.StaticCPUManager {
		t.Fatal("expected StaticCPUManager=true")
	}
}

func TestProbeNodeStaticMemoryManager(t *testing.T) {
	kubeDir := t.TempDir()
	kubeletCfg := filepath.Join(kubeDir, "kubelet-config.yaml")
	os.WriteFile(kubeletCfg, []byte("memoryManagerPolicy: static\n"), 0644)

	c, err := ProbeNode(t.TempDir(), kubeletCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !c.StaticMemoryManager {
		t.Fatal("expected StaticMemoryManager=true")
	}
}

func TestProbeNodeMissingKubeletConfig(t *testing.T) {
	c, err := ProbeNode(t.TempDir(), filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatal(err)
	}
	if c.StaticCPUManager || c.StaticMemoryManager {
		t.Fatal("expected no static managers without kubelet config")
	}
}

func TestProbeNodeCombined(t *testing.T) {
	cgroupDir := t.TempDir()
	f1, _ := os.Create(filepath.Join(cgroupDir, "cgroup.controllers"))
	f1.Close()
	f2, _ := os.Create(filepath.Join(cgroupDir, "memory.swap.events"))
	f2.Close()

	kubeDir := t.TempDir()
	kubeletCfg := filepath.Join(kubeDir, "kubelet-config.yaml")
	os.WriteFile(kubeletCfg, []byte("cpuManagerPolicy: static\nmemoryManagerPolicy: static\n"), 0644)

	c, err := ProbeNode(cgroupDir, kubeletCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !c.CgroupV2 {
		t.Fatal("expected CgroupV2=true")
	}
	if !c.SwapAccounting {
		t.Fatal("expected SwapAccounting=true")
	}
	if !c.StaticCPUManager {
		t.Fatal("expected StaticCPUManager=true")
	}
	if !c.StaticMemoryManager {
		t.Fatal("expected StaticMemoryManager=true")
	}
}
