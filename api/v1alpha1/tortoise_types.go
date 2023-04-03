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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
// Important: Run "make" to regenerate code after modifying this file

// TortoiseSpec defines the desired state of Tortoise
type TortoiseSpec struct {
	// TargetRefs has reference to involved resources.
	TargetRefs TargetRefs `json:"targetRefs" protobuf:"bytes,1,name=targetRefs"`
	// UpdateMode is how tortoise update resources.
	// If "Off", tortoise generates the recommendations in .Status, but doesn't apply it actually.
	// If "Auto", tortoise generates the recommendations in .Status, and apply it to resources.
	// If "Emergency", tortoise generates the recommendations in .Status as usual, but increase replica number high enough value.
	// "Emergency" is useful when something unexpected happens in workloads.
	//
	// "Off" is the default value.
	// +optional
	UpdateMode UpdateMode `json:"updateMode,omitempty" protobuf:"bytes,2,opt,name=updateMode"`
	// ResourcePolicy contains the policy how each resource is updated.
	ResourcePolicy []ContainerResourcePolicy `json:"resourcePolicy" protobuf:"bytes,3,name=resourcePolicy"`
	// FeatureGates allows to list the alpha feature names.
	// +optional
	FeatureGates []string `json:"featureGates,omitempty" protobuf:"bytes,4,opt,name=featureGates"`
}

type ContainerResourcePolicy struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// MinAllocatedResources is the minimum amount of resources which is given to the container.
	// Tortoise never set the resources request on the container than MinAllocatedResources.
	//
	// If empty, tortoise may reduce the resource request to the value which is suggested from VPA.
	// Leaving this field empty is basically safe, but you may consider using MinAllocatedResources when maybe your application will consume resources more than usual,
	// given the VPA suggests values based on the historical resource usage.
	// For example, your application will soon have new feature which leads to increase in the resource usage,
	// it is expected that your application will soon get more requests than usual, etc.
	// +optional
	MinAllocatedResources v1.ResourceList `json:"MinAllocatedResources" protobuf:"bytes,2,name=MinAllocatedResources"`
	// AutoscalingPolicy specifies how each resource is scaled.
	// If "Horizontal", the resource is horizontally scaled.
	// If "Vertical", the resource is vertically scaled.
	// Now, at least one container in Pod should be Horizontal.
	//
	// The default value is "Horizontal" for cpu, and "Vertical" for memory.
	// +optional
	AutoscalingPolicy map[v1.ResourceName]AutoscalingType `json:"autoscalingPolicy,omitempty" protobuf:"bytes,3,opt,name=autoscalingPolicy"`
}

// +kubebuilder:validation:Enum=Off;Auto;Emergency
type UpdateMode string

const (
	UpdateModeOff       UpdateMode = "Off"
	UpdateModeEmergency UpdateMode = "Emergency"
	AutoUpdateMode      UpdateMode = "Auto"
)

// +kubebuilder:validation:Enum=Horizontal;Vertical
type AutoscalingType string

const (
	AutoscalingTypeHorizontal AutoscalingType = "Horizontal"
	AutoscalingTypeVertical   AutoscalingType = "Vertical"
)

type TargetRefs struct {
	// DeploymentName is the name of target deployment.
	// It should be the same as the target of HPA.
	DeploymentName string `json:"deploymentName" protobuf:"bytes,1,name=deploymentName"`
	// HorizontalPodAutoscalerName is the name of the target HPA.
	// The target of this HPA should be the same as the DeploymentName above.
	// The target HPA should have the ContainerResource type metric or the external metric refers to the container resource utilization.
	// Please check out the document for more detail: https://github.com/mercari/tortoise/blob/master/docs/horizontal.md#supported-metrics-in-hpa
	//
	// If nothing specified, the Tortoise will create the HPA named "{Tortoise name} + -hpa" with needed ContainerResource type metric.
	// +optional
	HorizontalPodAutoscalerName *string `json:"horizontalPodAutoscalerName,omitempty" protobuf:"bytes,2,opt,name=horizontalPodAutoscalerName"`
}

// TortoiseStatus defines the observed state of Tortoise
type TortoiseStatus struct {
	TortoisePhase   TortoisePhase   `json:"tortoisePhase" protobuf:"bytes,1,name=tortoisePhase"`
	Conditions      Conditions      `json:"conditions" protobuf:"bytes,2,name=conditions"`
	Recommendations Recommendations `json:"recommendations" protobuf:"bytes,3,name=recommendations"`
	Targets         TargetsStatus   `json:"targets" protobuf:"bytes,4,name=targets"`
}

type TargetsStatus struct {
	HorizontalPodAutoscaler string                              `json:"horizontalPodAutoscaler" protobuf:"bytes,1,name=horizontalPodAutoscaler"`
	Deployment              string                              `json:"deployment" protobuf:"bytes,2,name=deployment"`
	VerticalPodAutoscalers  []TargetStatusVerticalPodAutoscaler `json:"verticalPodAutoscalers" protobuf:"bytes,3,name=verticalPodAutoscalers"`
}

type TargetStatusVerticalPodAutoscaler struct {
	Name string                    `json:"name" protobuf:"bytes,1,name=name"`
	Role VerticalPodAutoscalerRole `json:"role" protobuf:"bytes,2,name=role"`
}

// +kubebuilder:validation:Enum=Updater;Monitor
type VerticalPodAutoscalerRole string

const (
	VerticalPodAutoscalerRoleUpdater = "Updater"
	VerticalPodAutoscalerRoleMonitor = "Monitor"
)

type Recommendations struct {
	Horizontal HorizontalRecommendations `json:"horizontal" protobuf:"bytes,1,name=horizontal"`
	Vertical   VerticalRecommendations   `json:"vertical" protobuf:"bytes,2,name=vertical"`
}

type VerticalRecommendations struct {
	ContainerResourceRecommendation []RecommendedContainerResources `json:"containerResourceRecommendation" protobuf:"bytes,1,name=containerResourceRecommendation"`
}

type RecommendedContainerResources struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// RecommendedResource is the recommendation calculated by the tortoise.
	// If AutoscalingPolicy is vertical, it's the same value as the VPA suggests.
	// If AutoscalingPolicy is horizontal, it's basically the same value as the current resource request.
	// But, when the number of replicas are too small or too large,
	// tortoise may try to increase/decrease the amount of resources given to the container,
	// so that the number of replicas won't be very small or very large.
	RecommendedResource v1.ResourceList `json:"RecommendedResource" protobuf:"bytes,2,name=containerName"`
}

type HorizontalRecommendations struct {
	TargetUtilizations []HPATargetUtilizationRecommendationPerContainer `json:"targetUtilizations" protobuf:"bytes,1,name=targetUtilizations"`
	// MaxReplicas has the recommendation of maxReplicas.
	// It contains the recommendations for each time slot.
	MaxReplicas []ReplicasRecommendation `json:"maxReplicas" protobuf:"bytes,2,name=maxReplicas"`
	// MinReplicas has the recommendation of minReplicas.
	// It contains the recommendations for each time slot.
	MinReplicas []ReplicasRecommendation `json:"minReplicas" protobuf:"bytes,3,name=minReplicas"`
}

type ReplicasRecommendation struct {
	// From represented in hour.
	From int `json:"from" protobuf:"variant,1,name=from"`
	// To represented in hour.
	To       int          `json:"to" protobuf:"variant,2,name=to"`
	WeekDay  time.Weekday `json:"weekDay" protobuf:"bytes,3,name=weekDay"`
	TimeZone string       `json:"timeZone" protobuf:"bytes,4,name=timeZone"`
	// Value is the recommendation value.
	Value     int32       `json:"value" protobuf:"variant,5,name=value"`
	UpdatedAt metav1.Time `json:"updatedAt" protobuf:"bytes,6,name=updatedAt"`
}

type TortoisePhase string

const (
	// TortoisePhaseGatheringData means tortoise is now gathering data and cannot make the accurate recommendations.
	TortoisePhaseGatheringData TortoisePhase = "GatheringData"
	// TortoisePhaseWorking means tortoise is making the recommendations.
	TortoisePhaseWorking TortoisePhase = "Working"
	// TortoiseBackToNormal means tortoise was in the emergency mode, and now it's coming back to the normal operation.
	// During TortoiseBackToNormal, the number of replicas of workloads are gradually reduced to the usual value.
	// TODO: implement this.
	TortoiseBackToNormal TortoisePhase = "Working"
)

type HPATargetUtilizationRecommendationPerContainer struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// TargetUtilization is the recommendation of targetUtilization of HPA.
	TargetUtilization map[v1.ResourceName]int32 `json:"targetUtilization" protobuf:"bytes,2,name=targetUtilization"`
}

type Conditions struct {
	ContainerRecommendationFromVPA []ContainerRecommendationFromVPA `json:"containerRecommendationFromVPA" protobuf:"bytes,1,name=containerRecommendationFromVPA"`
}

type ContainerRecommendationFromVPA struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// MaxRecommendation is the max recommendation value from VPA among certain period (1 week).
	// Tortoise generates all recommendation based on this MaxRecommendation.
	MaxRecommendation map[v1.ResourceName]ResourceQuantity `json:"maxRecommendation" protobuf:"bytes,2,name=maxRecommendation"`
	// Recommendation is the latest recommendation value from VPA.
	Recommendation map[v1.ResourceName]ResourceQuantity `json:"recommendation" protobuf:"bytes,3,name=recommendation"`
}

type ResourceQuantity struct {
	Quantity  resource.Quantity `json:"quantity" protobuf:"bytes,1,name=quantity"`
	UpdatedAt metav1.Time       `json:"updatedAt" protobuf:"bytes,2,name=updatedAt"`
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
