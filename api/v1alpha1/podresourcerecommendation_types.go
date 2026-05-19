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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Common condition types written to PodResourceRecommendation.status.conditions.
const (
	PRRConditionAnomalous           = "Anomalous"
	PRRConditionInfeasible          = "Infeasible"
	PRRConditionPodLevelUnsupported = "PodLevelUnsupported"
)

// PodResourceRecommendationSpec links the recommendation to a specific pod.
type PodResourceRecommendationSpec struct {
	// TargetPodName is the pod this PRR shadows.
	TargetPodName string `json:"targetPodName"`
}

// PodLevelContention captures pod-level contention high-watermarks.
type PodLevelContention struct {
	// +optional
	Memory string `json:"memory,omitempty"`
	// +optional
	CPU string `json:"cpu,omitempty"`
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// ContainerLevelContention captures per-container contention high-watermarks.
type ContainerLevelContention struct {
	ContainerName string `json:"containerName"`
	// +optional
	Memory string `json:"memory,omitempty"`
	// +optional
	CPU string `json:"cpu,omitempty"`
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// ContentionHighWatermarks records decayed per-PRR high-watermarks.
type ContentionHighWatermarks struct {
	// +optional
	PodLevel *PodLevelContention `json:"podLevel,omitempty"`
	// +optional
	ContainerLevel []ContainerLevelContention `json:"containerLevel,omitempty"`
}

// ObservedPeakPodLevel is the absolute peak observed at the pod level.
type ObservedPeakPodLevel struct {
	// +optional
	Memory string `json:"memory,omitempty"`
}

// ObservedPeakContainer is the absolute peak observed for a single container.
type ObservedPeakContainer struct {
	ContainerName string `json:"containerName"`
	// +optional
	Memory string `json:"memory,omitempty"`
}

// ObservedPeak captures absolute peaks (memory floor enforcement).
type ObservedPeak struct {
	// +optional
	PodLevel *ObservedPeakPodLevel `json:"podLevel,omitempty"`
	// +optional
	Containers []ObservedPeakContainer `json:"containers,omitempty"`
}

// PodResourceRecommendationStatus is the recommender output and checkpoint.
type PodResourceRecommendationStatus struct {
	// +optional
	ContentionHighWatermarks *ContentionHighWatermarks `json:"contentionHighWatermarks,omitempty"`
	// +optional
	ObservedPeak *ObservedPeak `json:"observedPeak,omitempty"`

	// +optional
	PodLevelTarget corev1.ResourceList `json:"podLevelTarget,omitempty"`
	// +optional
	ContainerRecommendations []ContainerRecommendation `json:"containerRecommendations,omitempty"`

	// HistogramCheckpoint is the base64-encoded gob of the node-agent's
	// decaying histogram for this pod.
	// +optional
	HistogramCheckpoint string `json:"histogramCheckpoint,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=prr,categories=autoscaling

// PodResourceRecommendation is the Schema for the podresourcerecommendations API.
type PodResourceRecommendation struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PodResourceRecommendationSpec `json:"spec"`

	// +optional
	Status PodResourceRecommendationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PodResourceRecommendationList contains a list of PodResourceRecommendation.
type PodResourceRecommendationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PodResourceRecommendation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodResourceRecommendation{}, &PodResourceRecommendationList{})
}
