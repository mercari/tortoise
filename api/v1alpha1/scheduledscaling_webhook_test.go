package v1alpha1

import (
	"testing"
)

func TestScheduledScaling_ValidateAtLeastOneScalingParameter(t *testing.T) {
	tests := []struct {
		name    string
		ss      *ScheduledScaling
		wantErr bool
	}{
		{
			name: "both fields present - should pass",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Strategy: Strategy{
						Static: StaticStrategy{
							MinimumMinReplicas:    int32Ptr(5),
							MinAllocatedResources: &ResourceRequirements{CPU: "500m", Memory: "1Gi"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "all three scaling parameters present - should pass",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Strategy: Strategy{
						Static: StaticStrategy{
							MinimumMinReplicas:    int32Ptr(5),
							MinAllocatedResources: &ResourceRequirements{CPU: "500m", Memory: "1Gi"},
							ContainerMinAllocatedResources: []ContainerResourceRequirements{
								{
									ContainerName: "sidecar",
									Resources:     ResourceRequirements{CPU: "100m", Memory: "128Mi"},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only minimumMinReplicas present - should pass",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Strategy: Strategy{
						Static: StaticStrategy{
							MinimumMinReplicas: int32Ptr(5),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only minAllocatedResources present - should pass",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Strategy: Strategy{
						Static: StaticStrategy{
							MinAllocatedResources: &ResourceRequirements{CPU: "500m", Memory: "1Gi"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "only containerMinAllocatedResources present - should pass",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Strategy: Strategy{
						Static: StaticStrategy{
							ContainerMinAllocatedResources: []ContainerResourceRequirements{
								{
									ContainerName: "app",
									Resources:     ResourceRequirements{CPU: "500m", Memory: "1Gi"},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "both fields missing - should fail",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Strategy: Strategy{
						Static: StaticStrategy{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ss.validateAtLeastOneScalingParameter()
			if (err != nil) != tt.wantErr {
				t.Errorf("ScheduledScaling.validateAtLeastOneScalingParameter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScheduledScaling_ValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		ss      *ScheduledScaling
		wantErr bool
	}{
		{
			name: "valid spec with both fields",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Schedule: Schedule{
						Type:     ScheduleTypeTime,
						StartAt:  "2024-01-15T10:00:00Z",
						FinishAt: "2024-01-15T18:00:00Z",
					},
					Strategy: Strategy{
						Static: StaticStrategy{
							MinimumMinReplicas:    int32Ptr(5),
							MinAllocatedResources: &ResourceRequirements{CPU: "500m", Memory: "1Gi"},
						},
					},
					TargetRefs: TargetRefs{
						TortoiseName: "test-tortoise",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec with only minimumMinReplicas",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Schedule: Schedule{
						Type:     ScheduleTypeTime,
						StartAt:  "2024-01-15T10:00:00Z",
						FinishAt: "2024-01-15T18:00:00Z",
					},
					Strategy: Strategy{
						Static: StaticStrategy{
							MinimumMinReplicas: int32Ptr(5),
						},
					},
					TargetRefs: TargetRefs{
						TortoiseName: "test-tortoise",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid spec - missing both scaling parameters",
			ss: &ScheduledScaling{
				Spec: ScheduledScalingSpec{
					Schedule: Schedule{
						Type:     ScheduleTypeTime,
						StartAt:  "2024-01-15T10:00:00Z",
						FinishAt: "2024-01-15T18:00:00Z",
					},
					Strategy: Strategy{
						Static: StaticStrategy{},
					},
					TargetRefs: TargetRefs{
						TortoiseName: "test-tortoise",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ss.validateSpec()
			if (err != nil) != tt.wantErr {
				t.Errorf("ScheduledScaling.validateSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper function to create int32 pointers
func int32Ptr(i int32) *int32 {
	return &i
}
