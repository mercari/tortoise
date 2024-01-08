package vpa

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	"k8s.io/client-go/tools/record"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
)

func TestMakeAllVerticalContainerResourcePhaseRunning(t *testing.T) {
	type args struct {
		tortoise *autoscalingv1beta3.Tortoise
	}
	tests := []struct {
		name string
		args args
		want *autoscalingv1beta3.Tortoise
	}{
		{
			name: "modified correctly",
			args: args{
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
					},
				},
			},
			want: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
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
			got := SetAllVerticalContainerResourcePhaseWorking(tt.args.tortoise, time.Now())

			// use diff to compare
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})); diff != "" {
				t.Fatalf("MakeAllVerticalContainerResourcePhaseRunning() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestVPAContainerResourcePolicy(t *testing.T) {
	now := metav1.NewTime(time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC))

	type args struct {
		ctx          context.Context
		initTortoise *autoscalingv1beta3.Tortoise
		tortoise     *autoscalingv1beta3.Tortoise
		now          time.Time
	}
	tests := []struct {
		name         string
		args         args
		initialVPA   *vpav1.VerticalPodAutoscaler
		want         *vpav1.VerticalPodAutoscaler
		wantTortoise *autoscalingv1beta3.Tortoise
		wantErr      bool
	}{
		{
			name: "modified correctly",
			args: args{
				ctx: context.Background(),
				initTortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						ResourcePolicy: []autoscalingv1beta3.ContainerResourcePolicy{
							{
								ContainerName: "app",
								MinAllocatedResources: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("0.5Gi"),
									v1.ResourceCPU:    resource.MustParse("0.5"),
								},
							},
							{
								ContainerName: "istio-proxy",
								MinAllocatedResources: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("0.5Gi"),
									v1.ResourceCPU:    resource.MustParse("0.5"),
								},
							},
						},
					},
				},
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						ResourcePolicy: []autoscalingv1beta3.ContainerResourcePolicy{
							{
								ContainerName: "app",
								MinAllocatedResources: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("1Gi"),
									v1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "istio-proxy",
								MinAllocatedResources: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("1Gi"),
									v1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
				},
				now: now.Time,
			},
			initialVPA: &vpav1.VerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vpa",
				},
				Spec: vpav1.VerticalPodAutoscalerSpec{
					ResourcePolicy: &vpav1.PodResourcePolicy{
						ContainerPolicies: []vpav1.ContainerResourcePolicy{
							{
								ContainerName: "app",
								MinAllowed: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("0.5Gi"),
									v1.ResourceCPU:    resource.MustParse("0.5"),
								},
							},
							{
								ContainerName: "istio-proxy",
								MinAllowed: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("0.5Gi"),
									v1.ResourceCPU:    resource.MustParse("0.5"),
								},
							},
						},
					},
				},
			},
			want: &vpav1.VerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vpa",
				},
				Spec: vpav1.VerticalPodAutoscalerSpec{
					ResourcePolicy: &vpav1.PodResourcePolicy{
						ContainerPolicies: []vpav1.ContainerResourcePolicy{
							{
								ContainerName: "app",
								MinAllowed: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("1Gi"),
									v1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "istio-proxy",
								MinAllowed: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("1Gi"),
									v1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
					},
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					ResourcePolicy: []autoscalingv1beta3.ContainerResourcePolicy{
						{
							ContainerName: "app",
							MinAllocatedResources: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
						},
						{
							ContainerName: "istio-proxy",
							MinAllocatedResources: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Service{
				c:        fake.NewSimpleClientset(tt.initialVPA), // import "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake".
				recorder: record.NewFakeRecorder(10),
			}
			initVPA, initTortoise, err := c.CreateTortoiseUpdaterVPA(tt.args.ctx, tt.args.initTortoise)
			if (err != nil) != tt.wantErr {
				t.Logf("CreateTortoiseUpdaterVPA error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.initialVPA != nil {
				if d := cmp.Diff(tt.initialVPA, initVPA); d != "" {
					t.Logf("CreateTortoiseUpdaterVPA UpdaterVPA diff = %v", d)
				}
			}
			if d := cmp.Diff(tt.args.initTortoise, initTortoise); d != "" {
				t.Logf("CreateTortoiseUpdaterVPA Tortoise diff = %v", d)
			}

			got, tortoise, err := c.UpdateVPAContainerResourcePolicy(tt.args.ctx, tt.args.tortoise)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateVPAContainerResourcePolicy error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantTortoise != nil {
				if d := cmp.Diff(tt.wantTortoise, tortoise); d != "" {
					t.Errorf("UpdateVPAContainerResourcePolicy tortoise diff = %v", d)
				}
			}
			if d := cmp.Diff(tt.want.Spec, got.Spec); d != "" {
				t.Errorf("UpdateVPAContainerResourcePolicy vpa diff = %v", d)
			}
		})
	}
}
