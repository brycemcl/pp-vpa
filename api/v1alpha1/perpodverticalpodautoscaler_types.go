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
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// UpdateMode defines how the PP-VPA actuates recommendations.
// +kubebuilder:validation:Enum=Off;Initial;InPlace;Recreate
type UpdateMode string

const (
	UpdateModeOff      UpdateMode = "Off"
	UpdateModeInitial  UpdateMode = "Initial"
	UpdateModeInPlace  UpdateMode = "InPlace"
	UpdateModeRecreate UpdateMode = "Recreate"
)

// InfeasibleUpdateBehavior controls the budgeted scale-up-then-evict fallback.
type InfeasibleUpdateBehavior struct {
	// MaxSurge bounds extra temporary pods used to inherit state safely.
	// +optional
	// +kubebuilder:default="25%"
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`

	// MaxUnavailable bounds the PDB-honoring eviction path.
	// +optional
	// +kubebuilder:default=0
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// UpdatePolicy defines how the controller responds to recommendation changes.
type UpdatePolicy struct {
	// +optional
	// +kubebuilder:default=InPlace
	UpdateMode UpdateMode `json:"updateMode,omitempty"`

	// AnomalyEvictionTimeoutSeconds is the window over which a pod must
	// exceed the aggregate upperBound before it is considered anomalous.
	// +optional
	// +kubebuilder:default=900
	AnomalyEvictionTimeoutSeconds int32 `json:"anomalyEvictionTimeoutSeconds,omitempty"`

	// SignificantChangeThresholdPercentage gates churn: only update a PRR
	// when the recommendation changes by more than this percentage.
	// +optional
	// +kubebuilder:default=10
	SignificantChangeThresholdPercentage int32 `json:"significantChangeThresholdPercentage,omitempty"`

	// +optional
	InfeasibleUpdateBehavior InfeasibleUpdateBehavior `json:"infeasibleUpdateBehavior,omitempty"`

	// BurstTimeoutSeconds aborts an eviction when a surge pod is stuck Pending.
	// +optional
	// +kubebuilder:default=600
	BurstTimeoutSeconds int32 `json:"burstTimeoutSeconds,omitempty"`
}

// PodLevelPolicy constrains pod-level resource recommendations.
type PodLevelPolicy struct {
	// +kubebuilder:validation:Enum=cpu;memory
	ResourceName corev1.ResourceName `json:"resourceName"`
	// +optional
	MinAllowed *resource.Quantity `json:"minAllowed,omitempty"`
	// +optional
	MaxAllowed *resource.Quantity `json:"maxAllowed,omitempty"`
}

// ContainerPolicy constrains a single container's recommendations.
type ContainerPolicy struct {
	// ContainerName matches the container; "*" matches all containers.
	ContainerName string `json:"containerName"`
	// +optional
	MinAllowed corev1.ResourceList `json:"minAllowed,omitempty"`
	// +optional
	MaxAllowed corev1.ResourceList `json:"maxAllowed,omitempty"`
}

// ResourcePolicy bounds the recommendations.
type ResourcePolicy struct {
	// +optional
	PodLevelPolicies []PodLevelPolicy `json:"podLevelPolicies,omitempty"`
	// +optional
	ContainerPolicies []ContainerPolicy `json:"containerPolicies,omitempty"`
}

// RecommenderPolicy tunes the histogram-based estimator.
type RecommenderPolicy struct {
	// +optional
	// +kubebuilder:default="90.0"
	TargetPercentile string `json:"targetPercentile,omitempty"`
	// +optional
	// +kubebuilder:default="50.0"
	LowerBoundPercentile string `json:"lowerBoundPercentile,omitempty"`
	// +optional
	// +kubebuilder:default="95.0"
	UpperBoundPercentile string `json:"upperBoundPercentile,omitempty"`

	// SafetyMarginPercentage is added as a static pad and enforces the
	// minimum memory peak floor.
	// +optional
	// +kubebuilder:default=15
	SafetyMarginPercentage int32 `json:"safetyMarginPercentage,omitempty"`
}

// CPUScalingThresholds controls CPU triggers.
type CPUScalingThresholds struct {
	// ScaleUpPSI is the PSI avg10 above which CPU scale-up fires.
	// +optional
	// +kubebuilder:default="10.0"
	ScaleUpPSI string `json:"scaleUpPSI,omitempty"`
	// ScaleDownBufferDropPercentage is the relative drop from the
	// high-watermark required to trigger CPU scale-down.
	// +optional
	// +kubebuilder:default=20
	ScaleDownBufferDropPercentage int32 `json:"scaleDownBufferDropPercentage,omitempty"`
}

// MemoryScalingThresholds controls memory triggers.
type MemoryScalingThresholds struct {
	// +optional
	// +kubebuilder:default="5.0"
	ScaleUpPSI string `json:"scaleUpPSI,omitempty"`
	// WatermarkDecayWindowHours slowly lowers the high-watermark if not
	// hit within the window.
	// +optional
	// +kubebuilder:default=24
	WatermarkDecayWindowHours int32 `json:"watermarkDecayWindowHours,omitempty"`
}

// ScalingThresholds bundles CPU and memory triggers.
type ScalingThresholds struct {
	// +optional
	CPU CPUScalingThresholds `json:"cpu,omitempty"`
	// +optional
	Memory MemoryScalingThresholds `json:"memory,omitempty"`
}

// PerPodVerticalPodAutoscalerSpec defines the desired state.
type PerPodVerticalPodAutoscalerSpec struct {
	// TargetRef points at the workload controller (usually a Deployment).
	TargetRef autoscalingv1.CrossVersionObjectReference `json:"targetRef"`

	// TargetReplicas is the HPA-driven steady-state replica count. The
	// /scale subresource maps Scale.spec.replicas here; the controller
	// reconciles Deployment.spec.replicas to TargetReplicas + Status.TemporaryReplicas
	// so HPA reads remain stable across surge cycles.
	// +optional
	TargetReplicas *int32 `json:"targetReplicas,omitempty"`

	// +optional
	UpdatePolicy UpdatePolicy `json:"updatePolicy,omitempty"`

	// +optional
	ResourcePolicy ResourcePolicy `json:"resourcePolicy,omitempty"`

	// +optional
	RecommenderPolicy RecommenderPolicy `json:"recommenderPolicy,omitempty"`

	// +optional
	ScalingThresholds ScalingThresholds `json:"scalingThresholds,omitempty"`
}

// ContainerRecommendation is the per-container output of the recommender.
type ContainerRecommendation struct {
	ContainerName string `json:"containerName"`
	// +optional
	LowerBound corev1.ResourceList `json:"lowerBound,omitempty"`
	// +optional
	Target corev1.ResourceList `json:"target,omitempty"`
	// UncappedTarget exposes the pure calculation pre-clamping.
	// +optional
	UncappedTarget corev1.ResourceList `json:"uncappedTarget,omitempty"`
	// +optional
	UpperBound corev1.ResourceList `json:"upperBound,omitempty"`
}

// PodLevelRecommendation is the per-pod-level output of the recommender.
type PodLevelRecommendation struct {
	// +optional
	LowerBound corev1.ResourceList `json:"lowerBound,omitempty"`
	// +optional
	Target corev1.ResourceList `json:"target,omitempty"`
	// +optional
	UncappedTarget corev1.ResourceList `json:"uncappedTarget,omitempty"`
	// +optional
	UpperBound corev1.ResourceList `json:"upperBound,omitempty"`
}

// Recommendation is the aggregated workload recommendation.
type Recommendation struct {
	// +optional
	PodLevelTarget corev1.ResourceList `json:"podLevelTarget,omitempty"`
	// +optional
	PodLevel PodLevelRecommendation `json:"podLevel,omitempty"`
	// +optional
	ContainerRecommendations []ContainerRecommendation `json:"containerRecommendations,omitempty"`
}

// PerPodVerticalPodAutoscalerStatus is observed state.
type PerPodVerticalPodAutoscalerStatus struct {
	// +optional
	DefaultRecommendation *Recommendation `json:"defaultRecommendation,omitempty"`

	// ActiveReplicas reflects the steady-state desired pod count.
	// +optional
	ActiveReplicas int32 `json:"activeReplicas,omitempty"`
	// TemporaryReplicas reflects in-flight maxSurge inflation.
	// +optional
	TemporaryReplicas int32 `json:"temporaryReplicas,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.targetReplicas,statuspath=.status.activeReplicas
// +kubebuilder:resource:scope=Namespaced,shortName=ppvpa,categories=autoscaling

// PerPodVerticalPodAutoscaler is the Schema for the perpodverticalpodautoscalers API.
type PerPodVerticalPodAutoscaler struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PerPodVerticalPodAutoscalerSpec `json:"spec"`

	// +optional
	Status PerPodVerticalPodAutoscalerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PerPodVerticalPodAutoscalerList contains a list of PerPodVerticalPodAutoscaler.
type PerPodVerticalPodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PerPodVerticalPodAutoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PerPodVerticalPodAutoscaler{}, &PerPodVerticalPodAutoscalerList{})
}
