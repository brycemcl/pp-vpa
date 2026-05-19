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

// Package cgroup resolves a pod UID to its cgroup v2 slice path. The systemd
// cgroup driver layout is canonical; the cgroupfs driver layout is also
// supported via fallback resolution.
//
// Systemd layout:
//
//	/sys/fs/cgroup/kubepods.slice/kubepods-<qos>.slice/kubepods-<qos>-pod<uid>.slice
//
// where <qos> is "burstable" or "besteffort"; guaranteed QoS pods sit
// directly under /sys/fs/cgroup/kubepods.slice/kubepods-pod<uid>.slice.
package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const defaultRoot = "/sys/fs/cgroup"

// Resolver locates a pod's cgroup slice path.
type Resolver struct {
	// Root defaults to "/sys/fs/cgroup".
	Root string
}

// PodPath returns the absolute slice path for a pod with the given UID and QoS.
// Returns the first candidate path that exists on disk.
func (r *Resolver) PodPath(uid string, qos corev1.PodQOSClass) (string, error) {
	root := r.Root
	if root == "" {
		root = defaultRoot
	}
	// systemd driver replaces '-' in UIDs.
	uidSafe := strings.ReplaceAll(uid, "-", "_")
	candidates := []string{}
	switch qos {
	case corev1.PodQOSGuaranteed:
		candidates = append(candidates,
			filepath.Join(root, "kubepods.slice", fmt.Sprintf("kubepods-pod%s.slice", uidSafe)),
			filepath.Join(root, "kubepods", fmt.Sprintf("pod%s", uid)),
		)
	case corev1.PodQOSBurstable:
		candidates = append(candidates,
			filepath.Join(root, "kubepods.slice", "kubepods-burstable.slice", fmt.Sprintf("kubepods-burstable-pod%s.slice", uidSafe)),
			filepath.Join(root, "kubepods", "burstable", fmt.Sprintf("pod%s", uid)),
		)
	case corev1.PodQOSBestEffort:
		candidates = append(candidates,
			filepath.Join(root, "kubepods.slice", "kubepods-besteffort.slice", fmt.Sprintf("kubepods-besteffort-pod%s.slice", uidSafe)),
			filepath.Join(root, "kubepods", "besteffort", fmt.Sprintf("pod%s", uid)),
		)
	default:
		return "", fmt.Errorf("unknown pod QoS %q", qos)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no cgroup slice found for pod %s", uid)
}

// PSIFile returns the path of a pressure file under a cgroup slice.
func PSIFile(slicePath, resource string) string {
	return filepath.Join(slicePath, fmt.Sprintf("%s.pressure", resource))
}

// MemoryEventsFile returns the cgroup v2 memory.events path for a slice.
func MemoryEventsFile(slicePath string) string {
	return filepath.Join(slicePath, "memory.events")
}
