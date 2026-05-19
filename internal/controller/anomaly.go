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

package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/brycemclachlan/pp-vpa/api/v1alpha1"
)

// DetectAnomalies compares each PRR's current usage against the aggregate
// upperBound and returns a list of PatchOp operations to update PRR status.
//
// For each PRR:
//   - If usage exceeds upperBound on any resource and AnomalyExceedSince is
//     nil, set AnomalyExceedSince to now.
//   - If usage exceeds upperBound and has done so for longer than
//     anomalyEvictionTimeoutSeconds, set the Anomalous condition to True.
//   - If usage is back within bounds, clear AnomalyExceedSince and set the
//     Anomalous condition to False.
func DetectAnomalies(
	prrs []autoscalingv1alpha1.PodResourceRecommendation,
	rec *autoscalingv1alpha1.Recommendation,
	timeoutSeconds int32,
	now time.Time,
) []PRRAnomalyPatch {
	if rec == nil || timeoutSeconds <= 0 {
		return nil
	}

	var patches []PRRAnomalyPatch

	for i := range prrs {
		prr := &prrs[i]
		exceeding := exceedsUpperBound(prr, rec)

		patch := PRRAnomalyPatch{
			Namespace: prr.Namespace,
			Name:      prr.Name,
		}

		if exceeding {
			// Usage is above upperBound.
			if prr.Status.AnomalyExceedSince == nil {
				// First time exceeding: record start time.
				ts := metav1.NewTime(now)
				patch.AnomalyExceedSince = &ts
			}

			// Check if the exceed duration has surpassed the timeout.
			if prr.Status.AnomalyExceedSince != nil {
				exceedDuration := now.Sub(prr.Status.AnomalyExceedSince.Time)
				if exceedDuration >= time.Duration(timeoutSeconds)*time.Second {
					patch.SetAnomalousTrue = true
				}
			}
		} else {
			// Usage is within bounds.
			if prr.Status.AnomalyExceedSince != nil {
				// Was exceeding, now back to normal: clear the timestamp.
				patch.ClearAnomalyExceedSince = true
			}
			// Always ensure condition is False when usage is within bounds.
			patch.SetAnomalousFalse = true
		}

		if patch.HasChanges() {
			patches = append(patches, patch)
		}
	}

	return patches
}

// PRRAnomalyPatch describes the status changes to apply to a single PRR.
type PRRAnomalyPatch struct {
	Namespace string
	Name      string

	AnomalyExceedSince      *metav1.Time
	ClearAnomalyExceedSince bool
	SetAnomalousTrue        bool
	SetAnomalousFalse       bool
}

// HasChanges returns true if the patch contains any modifications.
func (p PRRAnomalyPatch) HasChanges() bool {
	return p.AnomalyExceedSince != nil || p.ClearAnomalyExceedSince || p.SetAnomalousTrue || p.SetAnomalousFalse
}

// exceedsUpperBound checks whether the PRR's current usage exceeds the
// aggregate upperBound on any resource.
func exceedsUpperBound(prr *autoscalingv1alpha1.PodResourceRecommendation, rec *autoscalingv1alpha1.Recommendation) bool {
	ub := rec.PodLevel.UpperBound

	// Check pod-level observed peak against pod-level upperBound.
	if prr.Status.ObservedPeak != nil && prr.Status.ObservedPeak.PodLevel != nil {
		if quantityExceedsBound(prr.Status.ObservedPeak.PodLevel.Memory, corev1.ResourceMemory, ub) {
			return true
		}
	}

	// Check pod-level contention watermarks against pod-level upperBound.
	if prr.Status.ContentionHighWatermarks != nil && prr.Status.ContentionHighWatermarks.PodLevel != nil {
		pl := prr.Status.ContentionHighWatermarks.PodLevel
		if quantityExceedsBound(pl.Memory, corev1.ResourceMemory, ub) {
			return true
		}
	}

	// Check per-container usage against container upperBounds.
	for _, cr := range prr.Status.ContainerRecommendations {
		ubCR := findContainerRec(rec.ContainerRecommendations, cr.ContainerName)
		if ubCR == nil {
			continue
		}
		if containerExceedsBound(cr, ubCR.UpperBound) {
			return true
		}
	}

	return false
}

// quantityExceedsBound checks if a quantity string exceeds the corresponding
// value in the upperBound ResourceList.
func quantityExceedsBound(quantityStr string, resourceName corev1.ResourceName, upperBound corev1.ResourceList) bool {
	if quantityStr == "" {
		return false
	}
	q, err := resource.ParseQuantity(quantityStr)
	if err != nil {
		return false
	}
	ub, ok := upperBound[resourceName]
	if !ok {
		return false
	}
	return q.Cmp(ub) > 0
}

// containerExceedsBound checks if any resource in a container's Target
// recommendation exceeds the given upperBound.
func containerExceedsBound(cr autoscalingv1alpha1.ContainerRecommendation, upperBound corev1.ResourceList) bool {
	for name, target := range cr.Target {
		if ub, ok := upperBound[name]; ok {
			if target.Cmp(ub) > 0 {
				return true
			}
		}
	}
	return false
}

// findContainerRec finds the container recommendation matching the given name.
func findContainerRec(recs []autoscalingv1alpha1.ContainerRecommendation, name string) *autoscalingv1alpha1.ContainerRecommendation {
	for i := range recs {
		if recs[i].ContainerName == name {
			return &recs[i]
		}
	}
	return nil
}
