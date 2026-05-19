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

package recommender

import (
	"math"
	"time"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Recommend builds a full Recommendation from a set of PRRs and a PP-VPA spec.
// It is intentionally pure so it can be tested without an API client.
func Recommend(
	prrs []autoscalingv1alpha1.PodResourceRecommendation,
	spec autoscalingv1alpha1.PerPodVerticalPodAutoscalerSpec,
	workloadAge time.Duration,
) (*autoscalingv1alpha1.Recommendation, error) {
	wh, err := Aggregate(prrs)
	if err != nil {
		return nil, err
	}
	b := ComputeBounds(wh, spec.RecommenderPolicy)
	b = ApplyConfidence(b, ConfidenceMultipliers(workloadAge))
	b = ApplyMargin(b, spec.RecommenderPolicy, maxObservedMemoryPeak(prrs))

	b = clampToPolicy(b, spec.ResourcePolicy)

	containerNames := uniqueContainerNames(prrs)
	containers := make([]autoscalingv1alpha1.ContainerRecommendation, 0, len(containerNames))
	for _, name := range containerNames {
		containers = append(containers, autoscalingv1alpha1.ContainerRecommendation{
			ContainerName:  name,
			LowerBound:     resourceList(b.CPULower, b.MemoryLower),
			Target:         resourceList(b.CPUTarget, b.MemoryTarget),
			UncappedTarget: resourceList(b.CPUUncapped, b.MemoryUncapped),
			UpperBound:     resourceList(b.CPUUpper, b.MemoryUpper),
		})
	}

	rec := &autoscalingv1alpha1.Recommendation{
		PodLevelTarget: podLevelList(b),
		PodLevel: autoscalingv1alpha1.PodLevelRecommendation{
			LowerBound:     resourceList(b.CPULower, b.MemoryLower),
			Target:         resourceList(b.CPUTarget, b.MemoryTarget),
			UncappedTarget: resourceList(b.CPUUncapped, b.MemoryUncapped),
			UpperBound:     resourceList(b.CPUUpper, b.MemoryUpper),
		},
		ContainerRecommendations: containers,
	}
	return rec, nil
}

func maxObservedMemoryPeak(prrs []autoscalingv1alpha1.PodResourceRecommendation) float64 {
	var peak float64
	for i := range prrs {
		op := prrs[i].Status.ObservedPeak
		if op == nil || op.PodLevel == nil || op.PodLevel.Memory == "" {
			continue
		}
		q, err := resource.ParseQuantity(op.PodLevel.Memory)
		if err != nil {
			continue
		}
		v := float64(q.Value())
		if v > peak {
			peak = v
		}
	}
	return peak
}

func uniqueContainerNames(prrs []autoscalingv1alpha1.PodResourceRecommendation) []string {
	seen := map[string]struct{}{}
	var out []string
	for i := range prrs {
		for _, c := range prrs[i].Status.ContainerRecommendations {
			if _, ok := seen[c.ContainerName]; ok {
				continue
			}
			seen[c.ContainerName] = struct{}{}
			out = append(out, c.ContainerName)
		}
	}
	return out
}

func resourceList(cpuCores, memBytes float64) corev1.ResourceList {
	if cpuCores < 0 {
		cpuCores = 0
	}
	if memBytes < 0 {
		memBytes = 0
	}
	cpuMilli := int64(math.Ceil(cpuCores * 1000))
	mem := int64(math.Ceil(memBytes))
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(cpuMilli, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(mem, resource.BinarySI),
	}
}

func podLevelList(b Bounds) corev1.ResourceList {
	return resourceList(b.CPUTarget, b.MemoryTarget)
}

func clampToPolicy(b Bounds, policy autoscalingv1alpha1.ResourcePolicy) Bounds {
	for _, p := range policy.PodLevelPolicies {
		switch p.ResourceName {
		case corev1.ResourceMemory:
			if p.MinAllowed != nil {
				v := float64(p.MinAllowed.Value())
				if b.MemoryTarget < v {
					b.MemoryTarget = v
				}
				if b.MemoryLower < v {
					b.MemoryLower = v
				}
			}
			if p.MaxAllowed != nil {
				v := float64(p.MaxAllowed.Value())
				if b.MemoryUpper > v {
					b.MemoryUpper = v
				}
				if b.MemoryTarget > v {
					b.MemoryTarget = v
				}
			}
		case corev1.ResourceCPU:
			if p.MinAllowed != nil {
				v := float64(p.MinAllowed.MilliValue()) / 1000.0
				if b.CPUTarget < v {
					b.CPUTarget = v
				}
				if b.CPULower < v {
					b.CPULower = v
				}
			}
			if p.MaxAllowed != nil {
				v := float64(p.MaxAllowed.MilliValue()) / 1000.0
				if b.CPUUpper > v {
					b.CPUUpper = v
				}
				if b.CPUTarget > v {
					b.CPUTarget = v
				}
			}
		}
	}
	return b
}
