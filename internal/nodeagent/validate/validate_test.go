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

package validate

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/brycemclachlan/pp-vpa/internal/compat"
)

func mustQuantity(s string) resource.Quantity {
	q := resource.MustParse(s)
	return q
}

func TestValidateResize(t *testing.T) {
	tests := []struct {
		name        string
		ctx         ResizeContext
		wantCodes   []string // expected violation codes (empty means valid)
		wantNoCodes []string // codes that must NOT appear
	}{
		{
			name: "valid resize Burstable Regular container",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: nil,
		},
		{
			name: "valid resize Guaranteed container",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSGuaranteed,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: nil,
		},
		{
			name: "valid resize SidecarInit container",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  SidecarInit,
				ContainerName:  "sidecar",
				CurrentCPU:     mustQuantity("50m"),
				CurrentMemory:  mustQuantity("64Mi"),
				ProposedCPU:    mustQuantity("100m"),
				ProposedMemory: mustQuantity("128Mi"),
			},
			wantCodes: nil,
		},
		{
			name: "BestEffort pod blocked",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBestEffort,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"QoSBestEffort"},
		},
		{
			name: "NonRestartableInit blocked",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  NonRestartableInit,
				ContainerName:  "setup",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"NonRestartableInit"},
		},
		{
			name: "Ephemeral container blocked",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Ephemeral,
				ContainerName:  "debugger",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"EphemeralContainer"},
		},
		{
			name: "CPU removed to zero blocked",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    resource.Quantity{},
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"CPURemoved"},
		},
		{
			name: "Memory removed to zero blocked",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: resource.Quantity{},
			},
			wantCodes: []string{"MemoryRemoved"},
		},
		{
			name: "Windows pod blocked",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				IsWindows:      true,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"WindowsPod"},
		},
		{
			name: "Static CPU manager blocks CPU resize",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{StaticCPUManager: true},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"StaticCPUManager"},
		},
		{
			name: "Static memory manager blocks memory resize",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{StaticMemoryManager: true},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"StaticMemoryManager"},
		},
		{
			name: "Swap accounting requires RestartContainer policy for memory",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{SwapAccounting: true},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
				ResizePolicies: []corev1.ContainerResizePolicy{
					{ResourceName: corev1.ResourceMemory, RestartPolicy: corev1.NotRequired},
				},
			},
			wantCodes: []string{"SwapNoRestartPolicy"},
		},
		{
			name: "Swap accounting passes with RestartContainer policy",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{SwapAccounting: true},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
				ResizePolicies: []corev1.ContainerResizePolicy{
					{ResourceName: corev1.ResourceMemory, RestartPolicy: corev1.RestartContainer},
				},
			},
			wantCodes: nil,
		},
		{
			name: "swap accounting does not flag CPU-only resize",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{SwapAccounting: true},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("128Mi"), // unchanged
			},
			wantCodes: nil,
		},
		{
			name: "combination: Windows + static CPU manager",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{StaticCPUManager: true},
				QoS:            corev1.PodQOSBurstable,
				IsWindows:      true,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"WindowsPod", "StaticCPUManager"},
		},
		{
			name: "combination: BestEffort + Ephemeral + Windows",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBestEffort,
				IsWindows:      true,
				ContainerType:  Ephemeral,
				ContainerName:  "debug",
				CurrentCPU:     mustQuantity("100m"),
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    mustQuantity("200m"),
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: []string{"QoSBestEffort", "EphemeralContainer", "WindowsPod"},
		},
		{
			name: "static CPU manager does not flag if no CPU resize",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{StaticCPUManager: true},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     resource.Quantity{},
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    resource.Quantity{},
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantNoCodes: []string{"StaticCPUManager"},
		},
		{
			name: "no violation when current and proposed are both zero for CPU",
			ctx: ResizeContext{
				NodeCaps:       compat.NodeCapabilities{},
				QoS:            corev1.PodQOSBurstable,
				ContainerType:  Regular,
				ContainerName:  "app",
				CurrentCPU:     resource.Quantity{},
				CurrentMemory:  mustQuantity("128Mi"),
				ProposedCPU:    resource.Quantity{},
				ProposedMemory: mustQuantity("256Mi"),
			},
			wantCodes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateResize(tt.ctx)
			gotCodes := codes(got)

			if len(tt.wantCodes) > 0 {
				for _, want := range tt.wantCodes {
					found := false
					for _, c := range gotCodes {
						if c == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected violation code %q, got %v", want, gotCodes)
					}
				}
			} else if len(got) > 0 {
				t.Errorf("expected no violations, got %v", gotCodes)
			}

			for _, noCode := range tt.wantNoCodes {
				for _, c := range gotCodes {
					if c == noCode {
						t.Errorf("unexpected violation code %q", noCode)
					}
				}
			}
		})
	}
}

func codes(vs []Violation) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Code
	}
	return out
}
