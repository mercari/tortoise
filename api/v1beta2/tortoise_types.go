/*
MIT License

Copyright (c) 2023 mercari

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/

package v1beta2

import (
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
	// "Emergency" is useful when something unexpected happens in workloads, and you want to scale up the workload with high enough resources.
	// See https://github.com/mercari/tortoise/blob/main/docs/emergency.md to know more about emergency mode.
	//
	// "Off" is the default value.
	// +optional
	UpdateMode UpdateMode `json:"updateMode,omitempty" protobuf:"bytes,2,opt,name=updateMode"`
	// ResourcePolicy contains the policy how each resource is updated.
	// +optional
	ResourcePolicy []ContainerResourcePolicy `json:"resourcePolicy,omitempty" protobuf:"bytes,3,opt,name=resourcePolicy"`
	// DeletionPolicy is the policy how the controller deletes associated HPA and VPAs when tortoise is removed.
	// If "DeleteAll", tortoise deletes all associated HPA and VPAs, created by tortoise. If the associated HPA is not created by tortoise,
	// which is associated by spec.targetRefs.horizontalPodAutoscalerName, tortoise never delete the HPA.
	// If "NoDelete", tortoise doesn't delete any associated HPA and VPAs.
	//
	// "NoDelete" is the default value.
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty" protobuf:"bytes,4,opt,name=deletionPolicy"`
	// AutoscalingPolicy is an optional field to specify how each resource in each container is scaled.
	//
	// You basically have two options to configure how each resource in each container is scaled -
	// 1. you let tortoise manage which resource is scaled with which autoscaling policy.
	// 2. you manage which resource is scaled with which autoscaling policy by yourself.
	//
	// For the first option, you just have to leave this field empty.
	// Then, when tortoise is created, tortoise will configure the AutoscalingPolicy for each resource in each container based on the following rules:
	// - If .spec.TargetRefs.HorizontalPodAutoscalerName is empty, the initial policy being used is "Horizontal" for cpu, and "Vertical" for memory.
	// - If .spec.TargetRefs.HorizontalPodAutoscalerName is not empty, tortoise sets "Horizontal" to resources managed by the attached HPA,
	//   and "Vertical" to resources not managed by the attached HPA.
	// Also, when a new container is added to the workload,
	// tortoise will notice it and configure the AutoscalingPolicy for each resource in the new container.
	// ("Horizontal" for cpu, and "Vertical" for memory)
	//
	// For the second option, you have to specify the AutoscalingPolicy for each resource in each container in this field.
	// If you specify the AutoscalingPolicy for some containers, but not for all,
	// tortoise will set the default value "Off" to the all resources in the containers not specified.
	// Note that when a new container is added to the workload, you have to update the AutoscalingPolicy field by yourself,
	// otherwise, tortoise will never scale the resources in the new container.
	//
	// Those two options are created for different use cases:
	// - Basically, you should use the first option that keep configuring the AutoscalingPolicy for each resource in each container automatically.
	// - If you have a special reason that you want to scale, for example, memory with horizontal scaling,
	//   you should use the second option that provides you the flexibility to configure the AutoscalingPolicy for each resource in each container.
	//
	// This field is mutable - you can even change this field from non-empty to empty.
	// In that case, tortoise will just keep the way each container is scaled,
	// but, you can obtain the merit of the first option - when a new container is added to the workload,
	// tortoise will notice it and configure the AutoscalingPolicy for each resource in the new container automatically.
	//
	// +optional
	AutoscalingPolicy []ContainerAutoscalingPolicy `json:"autoscalingPolicy,omitempty" protobuf:"bytes,5,opt,name=autoscalingPolicy"`
}

type ContainerAutoscalingPolicy struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// Policy specifies how each resource is scaled.
	// See .spec.AutoscalingPolicy for more defail.
	Policy map[v1.ResourceName]AutoscalingType `json:"policy,omitempty" protobuf:"bytes,2,opt,name=policy"`
}

type ContainerResourcePolicy struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// MinAllocatedResources is the minimum amount of resources which is given to the container.
	// Tortoise never set the resources request on the container than MinAllocatedResources.
	//
	// If empty, tortoise may reduce the resource request to the value which is suggested from VPA.
	// Given the VPA suggests values based on the historical resource usage,
	// you have no choice but to use MinAllocatedResources to pre-scaling your Pods,
	// for example, when maybe your application change will result in consuming resources more than the past.
	// +optional
	MinAllocatedResources v1.ResourceList `json:"minAllocatedResources,omitempty" protobuf:"bytes,2,opt,name=minAllocatedResources"`
	// AutoscalingPolicy specifies how each resource is scaled.
	// If "Horizontal", the resource is horizontally scaled.
	// If "Vertical", the resource is vertically scaled.
	// If "Off", tortoise doesn't scale at all based on that resource.
	//
	// If .spec.TargetRefs.HorizontalPodAutoscalerName is empty, the default value is "Horizontal" for cpu, and "Vertical" for memory.
	// If .spec.TargetRefs.HorizontalPodAutoscalerName is not empty, by default, it sets "Horizontal" to resources managed by the attached HPA,
	// and "Vertical" to resources not managed by the attached HPA.
	//
	// Deprecated: Use spec.AutoscalingPolicy instead.
	// It's now the alias of .spec.AutoscalingPolicy, and will be removed in a future version.
	// If both fields are specified, this field's value is just ignored.
	//
	// +optional
	DeplicatedAutoscalingPolicy map[v1.ResourceName]AutoscalingType `json:"autoscalingPolicy,omitempty" protobuf:"bytes,3,opt,name=autoscalingPolicy"`
}

// +kubebuilder:validation:Enum=DeleteAll;NoDelete
type DeletionPolicy string

const (
	DeletionPolicyDeleteAll DeletionPolicy = "DeleteAll"
	DeletionPolicyNoDelete  DeletionPolicy = "NoDelete"
)

// +kubebuilder:validation:Enum=Off;Auto;Emergency
type UpdateMode string

const (
	UpdateModeOff       UpdateMode = "Off"
	UpdateModeEmergency UpdateMode = "Emergency"
	UpdateModeAuto      UpdateMode = "Auto"
)

// +kubebuilder:validation:Enum=Off;Horizontal;Vertical
type AutoscalingType string

const (
	AutoscalingTypeOff        AutoscalingType = "Off"
	AutoscalingTypeHorizontal AutoscalingType = "Horizontal"
	AutoscalingTypeVertical   AutoscalingType = "Vertical"
)

type TargetRefs struct {
	// ScaleTargetRef is the target of scaling.
	// It should be the same as the target of HPA.
	ScaleTargetRef CrossVersionObjectReference `json:"scaleTargetRef" protobuf:"bytes,1,name=scaleTargetRef"`
	// HorizontalPodAutoscalerName is the name of the target HPA.
	// The target of this HPA should be the same as the ScaleTargetRef above.
	// The target HPA should have the ContainerResource type metric that refers to the container resource utilization.
	// Please check out the document for more detail: https://github.com/mercari/tortoise/blob/master/docs/horizontal.md#supported-metrics-in-hpa
	// Also, note that you must not edit the HPA directly after you attach the HPA to the tortoise of Auto mode.
	// Even if you edit your HPA in that case, tortoise will overwrite the HPA with the metrics/values.
	//
	// You can specify either of existing HPA only.
	// This is an optional field, and if you don't specify this field, tortoise will create a new default HPA named `tortoise-hpa-{tortoise name}`.
	// +optional
	HorizontalPodAutoscalerName *string `json:"horizontalPodAutoscalerName,omitempty" protobuf:"bytes,2,opt,name=horizontalPodAutoscalerName"`
}

// CrossVersionObjectReference contains enough information toet identify the referred resource.
type CrossVersionObjectReference struct {
	// kind is the kind of the referent; More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind" protobuf:"bytes,1,opt,name=kind"`

	// name is the name of the referent; More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`

	// apiVersion is the API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
}

// TortoiseStatus defines the observed state of Tortoise
type TortoiseStatus struct {
	TortoisePhase           TortoisePhase             `json:"tortoisePhase" protobuf:"bytes,1,name=tortoisePhase"`
	Conditions              Conditions                `json:"conditions" protobuf:"bytes,2,name=conditions"`
	Recommendations         Recommendations           `json:"recommendations" protobuf:"bytes,3,name=recommendations"`
	Targets                 TargetsStatus             `json:"targets" protobuf:"bytes,4,name=targets"`
	ContainerResourcePhases []ContainerResourcePhases `json:"containerResourcePhases" protobuf:"bytes,5,name=containerResourcePhases"`
	// AutoscalingPolicy contains the policy how this tortoise actually scales each resource.
	// It should basically be the same as .spec.autoscalingPolicy.
	// But, if .spec.autoscalingPolicy is empty, tortoise manages/generates
	// the policies generated based on HPA and the target deployment.
	AutoscalingPolicy []ContainerAutoscalingPolicy `json:"autoscalingPolicy,omitempty" protobuf:"bytes,6,opt,name=autoscalingPolicy"`
}

type ContainerResourcePhases struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// ResourcePhases is the phase of each resource of this container.
	ResourcePhases map[v1.ResourceName]ResourcePhase `json:"resourcePhases" protobuf:"bytes,2,name=resourcePhases"`
}

type ResourcePhase struct {
	Phase ContainerResourcePhase `json:"phase" protobuf:"bytes,1,name=phase"`
	// lastTransitionTime is the last time the condition transitioned from
	// one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,2,opt,name=lastTransitionTime"`
}

type ContainerResourcePhase string

const (
	ContainerResourcePhaseGatheringData ContainerResourcePhase = "GatheringData"
	ContainerResourcePhaseWorking       ContainerResourcePhase = "Working"
	ContainerResourcePhaseOff           ContainerResourcePhase = "Off"
)

type TortoisePhase string

const (
	// TortoisePhaseInitializing means tortoise is just created and initializing some components (HPA and VPAs),
	// and wait for those components to be ready.
	TortoisePhaseInitializing TortoisePhase = "Initializing"
	// TortoisePhaseGatheringData means tortoise is now gathering data and cannot make the accurate recommendations.
	TortoisePhaseGatheringData TortoisePhase = "GatheringData"
	// TortoisePhaseWorking means tortoise is making the recommendations,
	// and applying the recommendation values.
	TortoisePhaseWorking TortoisePhase = "Working"
	// TortoisePhasePartlyWorking means tortoise has maxReplicas and minReplicas recommendations ready,
	// and applying the recommendation values.
	// But, some of the resources are not scaled due to some reasons. (probably still gathering data)
	TortoisePhasePartlyWorking TortoisePhase = "PartlyWorking"
	// TortoisePhaseEmergency means tortoise is in the emergency mode.
	TortoisePhaseEmergency TortoisePhase = "Emergency"
	// TortoisePhaseBackToNormal means tortoise was in the emergency mode, and now it's coming back to the normal operation.
	// During TortoisePhaseBackToNormal, the number of replicas of workloads are gradually reduced to the usual value.
	TortoisePhaseBackToNormal TortoisePhase = "BackToNormal"
)

type TargetsStatus struct {
	// +optional
	HorizontalPodAutoscaler string                              `json:"horizontalPodAutoscaler" protobuf:"bytes,1,opt,name=horizontalPodAutoscaler"`
	ScaleTargetRef          CrossVersionObjectReference         `json:"scaleTargetRef" protobuf:"bytes,2,name=scaleTargetRef"`
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
	// +optional
	Horizontal HorizontalRecommendations `json:"horizontal,omitempty" protobuf:"bytes,1,opt,name=horizontal"`
	// +optional
	Vertical VerticalRecommendations `json:"vertical,omitempty" protobuf:"bytes,2,opt,name=vertical"`
}

type VerticalRecommendations struct {
	// +optional
	ContainerResourceRecommendation []RecommendedContainerResources `json:"containerResourceRecommendation" protobuf:"bytes,1,name=containerResourceRecommendation"`
}

type RecommendedContainerResources struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// RecommendedResource is the recommendation calculated by the tortoise.
	//
	// If AutoscalingPolicy is vertical, it's the same value as the VPA suggests.
	// If AutoscalingPolicy is horizontal, it's basically the same value as the current resource request.
	// But, when the number of replicas are too small or too large,
	// tortoise may try to increase/decrease the amount of resources given to the container,
	// so that the number of replicas won't be very small or very large.
	RecommendedResource v1.ResourceList `json:"RecommendedResource" protobuf:"bytes,2,name=containerName"`
}

type HorizontalRecommendations struct {
	// +optional
	TargetUtilizations []HPATargetUtilizationRecommendationPerContainer `json:"targetUtilizations,omitempty" protobuf:"bytes,1,opt,name=targetUtilizations"`
	// MaxReplicas has the recommendation of maxReplicas.
	// It contains the recommendations for each time slot.
	// +optional
	MaxReplicas []ReplicasRecommendation `json:"maxReplicas,omitempty" protobuf:"bytes,2,opt,name=maxReplicas"`
	// MinReplicas has the recommendation of minReplicas.
	// It contains the recommendations for each time slot.
	// +optional
	MinReplicas []ReplicasRecommendation `json:"minReplicas,omitempty" protobuf:"bytes,3,opt,name=minReplicas"`
}

type ReplicasRecommendation struct {
	// From represented in hour.
	From int `json:"from" protobuf:"variant,1,name=from"`
	// To represented in hour.
	To int `json:"to" protobuf:"variant,2,name=to"`
	// WeekDay is the day of the week.
	// If empty, it means it applies to all days of the week.
	WeekDay  *string `json:"weekday,omitempty" protobuf:"bytes,3,opt,name=weekday"`
	TimeZone string  `json:"timezone" protobuf:"bytes,4,name=timezone"`
	// Value is the recommendation value.
	// It's calculated every reconciliation,
	// and updated if the calculated recommendation value is more than the current recommendation value on tortoise.
	Value int32 `json:"value" protobuf:"variant,5,name=value"`
	// +optional
	UpdatedAt metav1.Time `json:"updatedAt,omitempty" protobuf:"bytes,6,opt,name=updatedAt"`
}

type HPATargetUtilizationRecommendationPerContainer struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// TargetUtilization is the recommendation of targetUtilization of HPA.
	TargetUtilization map[v1.ResourceName]int32 `json:"targetUtilization" protobuf:"bytes,2,name=targetUtilization"`
}

type Conditions struct {
	// TortoiseConditions is the condition of this tortoise.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	TortoiseConditions []TortoiseCondition `json:"tortoiseConditions" protobuf:"bytes,1,name=tortoiseConditions"`
	// ContainerRecommendationFromVPA is the condition of container recommendation from VPA, which is observed last time.
	// +optional
	ContainerRecommendationFromVPA []ContainerRecommendationFromVPA `json:"containerRecommendationFromVPA,omitempty" protobuf:"bytes,1,opt,name=containerRecommendationFromVPA"`
}

// TortoiseConditionType are the valid conditions of a Tortoise.
type TortoiseConditionType string

const (
	// TortoiseConditionTypeFailedToReconcile means tortoise failed to reconcile due to some reasons.
	TortoiseConditionTypeFailedToReconcile           TortoiseConditionType = "FailedToReconcile"
	TortoiseConditionTypeHPATargetUtilizationUpdated TortoiseConditionType = "HPATargetUtilizationUpdated"
)

type TortoiseCondition struct {
	// Type is the type of the condition.
	Type TortoiseConditionType `json:"type" protobuf:"bytes,1,name=type"`
	// Status is the status of the condition. (True, False, Unknown)
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,name=status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty" protobuf:"bytes,6,opt,name=lastUpdateTime"`
	// lastTransitionTime is the last time the condition transitioned from
	// one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// reason is the reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// message is a human-readable explanation containing details about
	// the transition
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

type ContainerRecommendationFromVPA struct {
	// ContainerName is the name of target container.
	ContainerName string `json:"containerName" protobuf:"bytes,1,name=containerName"`
	// MaxRecommendation is the max recommendation value from VPA in a certain period (1 week).
	// Tortoise generates all recommendation based on this MaxRecommendation.
	MaxRecommendation map[v1.ResourceName]ResourceQuantity `json:"maxRecommendation" protobuf:"bytes,2,name=maxRecommendation"`
	// Recommendation is the recommendation value from VPA that the tortoise controller observed in the last reconciliation..
	Recommendation map[v1.ResourceName]ResourceQuantity `json:"recommendation" protobuf:"bytes,3,name=recommendation"`
}

type ResourceQuantity struct {
	// +optional
	Quantity resource.Quantity `json:"quantity,omitempty" protobuf:"bytes,1,opt,name=quantity"`
	// +optional
	UpdatedAt metav1.Time `json:"updatedAt,omitempty" protobuf:"bytes,2,opt,name=updatedAt"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="MODE",type="string",JSONPath=".spec.updateMode"
//+kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.tortoisePhase"

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
