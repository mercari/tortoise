package vpa

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mercari/tortoise/api/v1beta2"
	autoscalingv1beta2 "github.com/mercari/tortoise/api/v1beta2"
)

func TestMakeAllVerticalContainerResourcePhaseRunning(t *testing.T) {
	type args struct {
		tortoise *autoscalingv1beta2.Tortoise
	}
	tests := []struct {
		name string
		args args
		want *autoscalingv1beta2.Tortoise
	}{
		{
			name: "modified correctly",
			args: args{
				tortoise: &autoscalingv1beta2.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta2.TortoiseSpec{
						ResourcePolicy: []autoscalingv1beta2.ContainerResourcePolicy{
							{
								ContainerName: "app",
								AutoscalingPolicy: map[v1.ResourceName]v1beta2.AutoscalingType{
									v1.ResourceMemory: v1beta2.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta2.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								AutoscalingPolicy: map[v1.ResourceName]v1beta2.AutoscalingType{
									v1.ResourceMemory: v1beta2.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta2.AutoscalingTypeHorizontal,
								},
							},
						},
					},
					Status: autoscalingv1beta2.TortoiseStatus{
						ContainerResourcePhases: []autoscalingv1beta2.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta2.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta2.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta2.ContainerResourcePhaseGatheringData,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta2.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta2.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta2.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
					},
				},
			},
			want: &autoscalingv1beta2.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta2.TortoiseSpec{
					ResourcePolicy: []autoscalingv1beta2.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[v1.ResourceName]v1beta2.AutoscalingType{
								v1.ResourceMemory: v1beta2.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta2.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							AutoscalingPolicy: map[v1.ResourceName]v1beta2.AutoscalingType{
								v1.ResourceMemory: v1beta2.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta2.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: autoscalingv1beta2.TortoiseStatus{
					ContainerResourcePhases: []autoscalingv1beta2.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta2.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta2.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta2.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta2.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta2.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta2.ContainerResourcePhaseWorking,
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeAllVerticalContainerResourcePhaseWorking(tt.args.tortoise, time.Now())

			// use diff to compare
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})); diff != "" {
				t.Fatalf("MakeAllVerticalContainerResourcePhaseRunning() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
