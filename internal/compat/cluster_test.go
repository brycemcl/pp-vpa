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
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestCapsFromVersion136Plus(t *testing.T) {
	c := capsFromVersion(&version.Info{Major: "1", Minor: "36"})
	if !c.PodLevelResources {
		t.Fatal("expected PodLevelResources=true for v1.36+")
	}
	if c.ContainerLevelOnly {
		t.Fatal("expected ContainerLevelOnly=false for v1.36+")
	}
}

func TestCapsFromVersion133To135(t *testing.T) {
	for _, minor := range []string{"33", "34", "35"} {
		c := capsFromVersion(&version.Info{Major: "1", Minor: minor})
		if c.PodLevelResources {
			t.Fatalf("expected PodLevelResources=false for v1.%s", minor)
		}
		if !c.ContainerLevelOnly {
			t.Fatalf("expected ContainerLevelOnly=true for v1.%s", minor)
		}
	}
}

func TestCapsFromVersionOlder(t *testing.T) {
	c := capsFromVersion(&version.Info{Major: "1", Minor: "32"})
	if c.PodLevelResources {
		t.Fatal("expected PodLevelResources=false for v1.32")
	}
	if c.ContainerLevelOnly {
		t.Fatal("expected ContainerLevelOnly=false for v1.32")
	}
}

func TestCapsFromVersionWithPlusSuffix(t *testing.T) {
	c := capsFromVersion(&version.Info{Major: "1", Minor: "35+"})
	if !c.ContainerLevelOnly {
		t.Fatal("expected ContainerLevelOnly=true for v1.35+")
	}
	if c.PodLevelResources {
		t.Fatal("expected PodLevelResources=false for v1.35+")
	}
}

func TestCapsFromVersionMinorZero(t *testing.T) {
	c := capsFromVersion(&version.Info{Major: "1", Minor: "0"})
	if c.PodLevelResources || c.ContainerLevelOnly {
		t.Fatal("expected both false for v1.0")
	}
}
