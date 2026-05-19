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

package cgroup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestResolverSystemdLayout(t *testing.T) {
	root := t.TempDir()
	uid := "abcd-1234-ef56"
	uidSafe := strings.ReplaceAll(uid, "-", "_")
	want := filepath.Join(root, "kubepods.slice", "kubepods-burstable.slice",
		"kubepods-burstable-pod"+uidSafe+".slice")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{Root: root}
	got, err := r.PodPath(uid, corev1.PodQOSBurstable)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("path mismatch: got %q want %q", got, want)
	}
}

func TestResolverNotFound(t *testing.T) {
	r := &Resolver{Root: t.TempDir()}
	if _, err := r.PodPath("missing", corev1.PodQOSGuaranteed); err == nil {
		t.Fatal("expected error for missing slice")
	}
}
