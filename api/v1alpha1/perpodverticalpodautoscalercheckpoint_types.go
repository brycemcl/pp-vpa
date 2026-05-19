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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PerPodVerticalPodAutoscalerCheckpointSpec links the checkpoint to its PP-VPA.
type PerPodVerticalPodAutoscalerCheckpointSpec struct {
	// PPVPARef is the parent PerPodVerticalPodAutoscaler.
	// +optional
	PPVPARef string `json:"ppvpaRef,omitempty"`
}

// PerPodVerticalPodAutoscalerCheckpointStatus stores the workload-wide histogram.
type PerPodVerticalPodAutoscalerCheckpointStatus struct {
	// AggregateHistogramCheckpoint is base64+gob of the aggregated decaying histogram.
	// +optional
	AggregateHistogramCheckpoint string `json:"aggregateHistogramCheckpoint,omitempty"`

	// LastUpdateTime tracks freshness.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ppvpacp,categories=autoscaling

// PerPodVerticalPodAutoscalerCheckpoint is the Schema for the PP-VPA checkpoint API.
type PerPodVerticalPodAutoscalerCheckpoint struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PerPodVerticalPodAutoscalerCheckpointSpec `json:"spec"`

	// +optional
	Status PerPodVerticalPodAutoscalerCheckpointStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PerPodVerticalPodAutoscalerCheckpointList contains a list.
type PerPodVerticalPodAutoscalerCheckpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PerPodVerticalPodAutoscalerCheckpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PerPodVerticalPodAutoscalerCheckpoint{}, &PerPodVerticalPodAutoscalerCheckpointList{})
}
