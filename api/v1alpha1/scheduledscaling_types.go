package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScheduledScalingSpec defines the desired state of ScheduledScaling
type ScheduledScalingSpec struct {
	// Schedule defines when the scaling should occur
	// +kubebuilder:validation:Required
	Schedule Schedule `json:"schedule"`

	// TargetRefs specifies which resources this scheduled scaling should affect
	// +kubebuilder:validation:Required
	TargetRefs TargetRefs `json:"targetRefs"`

	// Strategy defines how the scaling should be performed
	// +kubebuilder:validation:Required
	Strategy Strategy `json:"strategy"`

	// Status indicates the current state of the scheduled scaling
	// +kubebuilder:validation:Enum=Inactive;Active
	// +kubebuilder:default=Inactive
	Status ScheduledScalingState `json:"status,omitempty"`
}

// Schedule defines the timing for scheduled scaling
type Schedule struct {
	// StartAt specifies when the scaling should begin
	// Format: RFC3339 (e.g., "2024-01-15T10:00:00Z")
	// +kubebuilder:validation:Required
	StartAt string `json:"startAt"`

	// FinishAt specifies when the scaling should end and return to normal
	// Format: RFC3339 (e.g., "2024-01-15T18:00:00Z")
	// +kubebuilder:validation:Required
	FinishAt string `json:"finishAt"`
}

// TargetRefs specifies which resources to scale
type TargetRefs struct {
	// TortoiseName is the name of the Tortoise resource to scale
	// +kubebuilder:validation:Required
	TortoiseName string `json:"tortoiseName"`
}

// Strategy defines how the scaling should be performed
type Strategy struct {
	// Static defines static scaling parameters
	// +kubebuilder:validation:Required
	Static StaticStrategy `json:"static"`
}

// StaticStrategy defines static scaling parameters
type StaticStrategy struct {
	// MinimumMinReplicas sets the minimum number of replicas during scaling
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	MinimumMinReplicas int32 `json:"minimumMinReplicas"`

	// MinAllocatedResources sets the minimum allocated resources during scaling
	// +kubebuilder:validation:Required
	MinAllocatedResources ResourceRequirements `json:"minAllocatedResources"`
}

// ResourceRequirements describes the compute resource requirements
type ResourceRequirements struct {
	// CPU specifies the CPU resource requirements
	// +kubebuilder:validation:Required
	CPU string `json:"cpu"`

	// Memory specifies the memory resource requirements
	// +kubebuilder:validation:Required
	Memory string `json:"memory"`
}

// ScheduledScalingState represents the desired state of a scheduled scaling operation
type ScheduledScalingState string

const (
	// ScheduledScalingStateInactive means the scheduled scaling is not active
	ScheduledScalingStateInactive ScheduledScalingState = "Inactive"
	// ScheduledScalingStateActive means the scheduled scaling is currently active
	ScheduledScalingStateActive ScheduledScalingState = "Active"
)

// ScheduledScalingStatus defines the observed state of ScheduledScaling
// +kubebuilder:object:generate=true
type ScheduledScalingStatus struct {
	// Phase indicates the current phase of the scheduled scaling
	// +kubebuilder:validation:Enum=Pending;Active;Completed;Failed
	Phase ScheduledScalingPhase `json:"phase,omitempty"`

	// LastTransitionTime is the last time the status transitioned from one phase to another
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Message provides additional information about the current phase
	Message string `json:"message,omitempty"`

	// Reason indicates why the scheduled scaling is in the current phase
	Reason string `json:"reason,omitempty"`
}

// ScheduledScalingPhase represents the phase of a scheduled scaling operation
type ScheduledScalingPhase string

const (
	// ScheduledScalingPhasePending means the scheduled scaling is waiting to start
	ScheduledScalingPhasePending ScheduledScalingPhase = "Pending"
	// ScheduledScalingPhaseActive means the scheduled scaling is currently active
	ScheduledScalingPhaseActive ScheduledScalingPhase = "Active"
	// ScheduledScalingPhaseCompleted means the scheduled scaling has completed successfully
	ScheduledScalingPhaseCompleted ScheduledScalingPhase = "Completed"
	// ScheduledScalingPhaseFailed means the scheduled scaling has failed
	ScheduledScalingPhaseFailed ScheduledScalingPhase = "Failed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Start Time",type="string",JSONPath=".spec.schedule.startAt"
//+kubebuilder:printcolumn:name="End Time",type="string",JSONPath=".spec.schedule.finishAt"
//+kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetRefs.tortoiseName"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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

func init() {
	SchemeBuilder.Register(&ScheduledScaling{}, &ScheduledScalingList{})
}
