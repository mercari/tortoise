/*
Copyright 2023.

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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TortoiseSpec defines the desired state of Tortoise
type TortoiseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	TargetRefs     TargetRefs
	UpdateMode     UpdateMode
	ResourcePolicy []ContainerResourcePolicy `json:"resourcePolicy,omitempty" protobuf:"bytes,3,opt,name=resourcePolicy"`
	// If enabled, tortoise works with the alpha feature(s).
	FeatureGates []string
}

type ContainerResourcePolicy struct {
	ContainerName       string
	MinAllowedResources v1.ResourceList
	AutoscalingPolicy   map[v1.ResourceName]AutoscalingType
}

// +kubebuilder:validation:Enum=Off;Auto
type UpdateMode string

const (
	OffUpdateMode  UpdateMode = "Off"
	AutoUpdateMode UpdateMode = "Auto"
)

// +kubebuilder:validation:Enum=Horizontal;Vertical
type AutoscalingType string

const (
	HorizontalAutoscalingType AutoscalingType = "Horizontal"
	VerticalAutoscalingType   AutoscalingType = "Vertical"
)

type TargetRefs struct {
	DeploymentRef              string
	HorizontalPodAutoscalerRef string
}

type VerticalPodAutoscalersPerContainerRef struct {
	ContainerName                        string
	VerticalPodAutoscalersPreResourceRef map[v1.ResourceName]string
}

// TortoiseStatus defines the observed state of Tortoise
type TortoiseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	TortoisePhase TortoisePhase

	Conditions Conditions

	Recommendations Recommendations
}

type Recommendations struct {
	HPA HPARecommendations
	VPA VPARecommendations
}

type VPARecommendations struct {
	ContainerResourceRecommendation []RecommendedContainerResources
}

type RecommendedContainerResources struct {
	ContainerName string
	Resource      v1.ResourceList
}

type HPARecommendations struct {
	TargetUtilizations []HPATargetUtilizationRecommendationPerContainer
	MaxReplicas        []ReplicasRecommendation
	MinReplicas        []ReplicasRecommendation
}

type ReplicasRecommendation struct {
	From      metav1.Time
	To        metav1.Time
	Value     int32
	UpdatedAt metav1.Time
}

type TortoisePhase string

const (
	TortoisePhaseUnknown       TortoisePhase = "Unknown"
	TortoisePhaseGatheringData TortoisePhase = "GatheringData"
	TortoisePhaseWorking       TortoisePhase = "Working"
)

type HPATargetUtilizationRecommendationPerContainer struct {
	ContainerName     string
	TargetUtilization map[v1.ResourceName]int32
}

type Conditions struct {
	ContainerRecommendationFromVPA []ContainerRecommendationFromVPA
}

type ContainerRecommendationFromVPA struct {
	ContainerName string
	// MaxRecommendation is the max recommendation value from VPA among certain period (1 week).
	// Tortoise generates all conguration based on this MaxRecommendation.
	// TODO: make the period configurable.
	MaxRecommendation map[v1.ResourceName]ResourceQuantity
	// Recommendation is the latest recommendation value from VPA.
	Recommendation map[v1.ResourceName]ResourceQuantity
}

type ResourceQuantity struct {
	Quantity  resource.Quantity
	UpdatedAt metav1.Time
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Tortoise is the Schema for the tortoises API
type Tortoise struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TortoiseSpec   `json:"spec,omitempty"`
	Status TortoiseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TortoiseList contains a list of Tortoise
type TortoiseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tortoise `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tortoise{}, &TortoiseList{})
}
