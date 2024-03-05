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

package v1alpha1

import (
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScheduledScalingSpec defines the desired state of ScheduledScaling
type ScheduledScalingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	/// Schedule RFC3339 format e.g., "2006-01-02T15:04:05Z09:00"
	Schedule Schedule `json:"schedule" protobuf:"bytes,1,name=schedule"`

	// TargetRef is the targets that need to be scheduled
	TargetRefs TargetRefs `json:"targetRefs" protobuf:"bytes,2,name=targetRefs"`

	// Strategy describes how this ScheduledScaling scales the target workload.
	// Currently, it only supports the static strategy.
	Strategy Strategy `json:"strategy" protobuf:"bytes,3,name=strategy"`
}

// ScheduledScalingStatus defines the observed state of ScheduledScaling
type ScheduledScalingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ScheduledScalingPhase ScheduledScalingPhase `json:"scheduledScalingPhase" protobuf:"bytes,1,name=scheduledScalingPhase"`
}

type ScheduledScalingPhase string

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ScheduledScaling is the Schema for the scheduledscalings API
type ScheduledScaling struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledScalingSpec   `json:"spec,omitempty"`
	Status ScheduledScalingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScheduledScalingList contains a list of ScheduledScaling
type ScheduledScalingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledScaling `json:"items"`
}

type TargetRefs struct {
	//Tortoise to be targeted for scheduled scaling
	TortoiseName *string `json:"tortoiseName,omitempty" protobuf:"bytes,2,name=tortoiseName"`
}

type Schedule struct {
	/// RFC3339 format e.g., "2006-01-02T15:04:05Z09:00"
	// start of schedule
	StartAt *string `json:"startAt,omitempty" protobuf:"bytes,1,opt,name=startAt"`
	// end of schedule
	FinishAt *string `json:"finishAt,omitempty" protobuf:"bytes,2,name=finishAt"`
}

type Strategy struct {
	// Resource scaling requirements
	// Static strategy receives the static value of the replica number and resource requests
	// that users want to give to Pods during this ScheduledScaling is valid.
	// This field is optional for the future when we add another strategy here though,
	// until then this strategy is the only supported strategy
	// you must set this.
	// +optional
	Static *Static `json:"static,omitempty" protobuf:"bytes,1,opt,name=static"`
}

type Static struct {
	// MinimumMinReplicas means the minimum MinReplicas that Tortoise gives to HPA during this ScheduledScaling is valid.
	MinimumMinReplicas *int `json:"minimumMinReplica,omitempty" protobuf:"bytes,1,opt,name=minimumMinReplica"`
	// MinAllowedResources means the minimum resource requests that Tortoise gives to each container.
	MinAllocatedResources []autoscalingv1beta3.ContainerResourceRequests `json:"minAllocatedResourcesomitempty,omitempty" protobuf:"bytes,2,opt,name=minAllocatedResources"`
}

func init() {
	SchemeBuilder.Register(&ScheduledScaling{}, &ScheduledScalingList{})
}
