package hpa

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appv1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autoscalingv1beta1 "github.com/mercari/tortoise/api/v1beta1"
	"github.com/mercari/tortoise/pkg/annotation"
)

func TestClient_UpdateHPAFromTortoiseRecommendation(t *testing.T) {
	now := metav1.NewTime(time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC))

	type args struct {
		ctx      context.Context
		tortoise *autoscalingv1beta1.Tortoise
		now      time.Time
	}
	tests := []struct {
		name         string
		args         args
		initialHPA   *v2.HorizontalPodAutoscaler
		want         *v2.HorizontalPodAutoscaler
		wantTortoise *autoscalingv1beta1.Tortoise
		wantErr      bool
	}{
		{
			name: "Basic test case with container resource metrics",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta1.Tortoise{
					Status: autoscalingv1beta1.TortoiseStatus{
						Targets: autoscalingv1beta1.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta1.Recommendations{
							Horizontal: autoscalingv1beta1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta1.HPATargetUtilizationRecommendationPerContainer{
									{
										ContainerName: "app",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceMemory: 90,
										},
									},
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceCPU: 80,
										},
									},
								},
								MaxReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
							},
						},
					},
				},
				now: now.Time,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			want: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(3),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "no update preformed when updateMode is Off",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta1.Tortoise{
					Spec: autoscalingv1beta1.TortoiseSpec{
						UpdateMode: autoscalingv1beta1.UpdateModeOff,
					},
					Status: autoscalingv1beta1.TortoiseStatus{
						Targets: autoscalingv1beta1.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta1.Recommendations{
							Horizontal: autoscalingv1beta1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta1.HPATargetUtilizationRecommendationPerContainer{
									{
										ContainerName: "app",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceMemory: 90,
										},
									},
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceCPU: 80,
										},
									},
								},
								MaxReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
							},
						},
					},
				},
				now: now.Time,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			want: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "emergency mode",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta1.Tortoise{
					Status: autoscalingv1beta1.TortoiseStatus{
						Targets: autoscalingv1beta1.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						TortoisePhase: autoscalingv1beta1.TortoisePhaseEmergency,
						Recommendations: autoscalingv1beta1.Recommendations{
							Horizontal: autoscalingv1beta1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta1.HPATargetUtilizationRecommendationPerContainer{
									{
										ContainerName: "app",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceMemory: 90,
										},
									},
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceCPU: 80,
										},
									},
								},
								MaxReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
							},
						},
					},
				},
				now: now.Time,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			want: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(6),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "minReplicas are reduced gradually during BackToNormal",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta1.Tortoise{
					Status: autoscalingv1beta1.TortoiseStatus{
						Targets: autoscalingv1beta1.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						TortoisePhase: autoscalingv1beta1.TortoisePhaseBackToNormal,
						Recommendations: autoscalingv1beta1.Recommendations{
							Horizontal: autoscalingv1beta1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta1.HPATargetUtilizationRecommendationPerContainer{
									{
										ContainerName: "app",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceMemory: 90,
										},
									},
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceCPU: 80,
										},
									},
								},
								MaxReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
							},
						},
					},
				},
				now: now.Time,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(6),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			want: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(5),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "BackToNormal finishes when minReplicas reaches the ideal value",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta1.Tortoise{
					Status: autoscalingv1beta1.TortoiseStatus{
						Targets: autoscalingv1beta1.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						TortoisePhase: autoscalingv1beta1.TortoisePhaseBackToNormal,
						Recommendations: autoscalingv1beta1.Recommendations{
							Horizontal: autoscalingv1beta1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta1.HPATargetUtilizationRecommendationPerContainer{
									{
										ContainerName: "app",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceMemory: 90,
										},
									},
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[v1.ResourceName]int32{
											v1.ResourceCPU: 80,
										},
									},
								},
								MaxReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   pointer.String(now.Weekday().String()),
									},
								},
							},
						},
					},
				},
				now: now.Time,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			want: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(3),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							// should be ignored
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: pointer.Int32(80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &autoscalingv1beta1.Tortoise{
				Status: autoscalingv1beta1.TortoiseStatus{
					Targets: autoscalingv1beta1.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					TortoisePhase: autoscalingv1beta1.TortoisePhaseWorking,
					Recommendations: autoscalingv1beta1.Recommendations{
						Horizontal: autoscalingv1beta1.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta1.HPATargetUtilizationRecommendationPerContainer{
								{
									ContainerName: "app",
									TargetUtilization: map[v1.ResourceName]int32{
										v1.ResourceMemory: 90,
									},
								},
								{
									ContainerName: "istio-proxy",
									TargetUtilization: map[v1.ResourceName]int32{
										v1.ResourceCPU: 80,
									},
								},
							},
							MaxReplicas: []autoscalingv1beta1.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   pointer.String(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta1.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   pointer.String(now.Weekday().String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90)
			got, tortoise, err := c.UpdateHPAFromTortoiseRecommendation(tt.args.ctx, tt.args.tortoise, tt.args.now)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateHPAFromTortoiseRecommendation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantTortoise != nil {
				if d := cmp.Diff(tt.wantTortoise, tortoise); d != "" {
					t.Errorf("UpdateHPAFromTortoiseRecommendation() tortoise diff = %v", d)
				}
			}
			if d := cmp.Diff(tt.want.Spec, got.Spec); d != "" {
				t.Errorf("UpdateHPAFromTortoiseRecommendation() hpa diff = %v", d)
			}
		})
	}
}

func ptrInt32(i int32) *int32 {
	return &i
}

func TestService_InitializeHPA(t *testing.T) {
	type args struct {
		tortoise *autoscalingv1beta1.Tortoise
		dm       *appv1.Deployment
	}
	tests := []struct {
		name       string
		initialHPA *v2.HorizontalPodAutoscaler
		args       args
		afterHPA   *v2.HorizontalPodAutoscaler
		wantErr    bool
	}{
		{
			name: "should create new hpa",
			args: args{
				tortoise: &autoscalingv1beta1.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta1.TortoiseSpec{
						TargetRefs: autoscalingv1beta1.TargetRefs{
							ScaleTargetRef: autoscalingv1beta1.CrossVersionObjectReference{
								Kind:       "Deployment",
								Name:       "deployment",
								APIVersion: "apps/v1",
							},
						},
					},
				},
				dm: &appv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment",
						Namespace: "default",
					},
					Spec: appv1.DeploymentSpec{
						Replicas: pointer.Int32(4),
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "app",
									},
								},
							},
						},
					},
					Status: appv1.DeploymentStatus{
						Replicas: 4,
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise-hpa-tortoise",
					Namespace: "default",
					Annotations: map[string]string{
						annotation.TortoiseNameAnnotation:      "tortoise",
						annotation.ManagedByTortoiseAnnotation: "true",
					},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(2),
					MaxReplicas: 8,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Behavior: &v2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &v2.HPAScalingRules{
							Policies: []v2.HPAScalingPolicy{
								{
									Type:          v2.PercentScalingPolicy,
									Value:         100,
									PeriodSeconds: 60,
								},
							},
						},
						ScaleDown: &v2.HPAScalingRules{
							Policies: []v2.HPAScalingPolicy{
								{
									Type:          v2.PercentScalingPolicy,
									Value:         2,
									PeriodSeconds: 90,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "just give annotation to existing hpa",
			args: args{
				tortoise: &autoscalingv1beta1.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta1.TortoiseSpec{
						TargetRefs: autoscalingv1beta1.TargetRefs{
							HorizontalPodAutoscalerName: pointer.String("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta1.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
				},
				dm: &appv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment",
						Namespace: "default",
					},
					Spec: appv1.DeploymentSpec{
						Replicas: pointer.Int32(4),
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name: "app",
									},
								},
							},
						},
					},
					Status: appv1.DeploymentStatus{
						Replicas: 4,
					},
				},
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "existing-hpa",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Behavior: &v2.HorizontalPodAutoscalerBehavior{
						ScaleDown: &v2.HPAScalingRules{
							Policies: []v2.HPAScalingPolicy{
								{
									Type:          v2.PercentScalingPolicy,
									Value:         2,
									PeriodSeconds: 90,
								},
							},
						},
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
					Annotations: map[string]string{
						annotation.TortoiseNameAnnotation:      "tortoise",
						annotation.ManagedByTortoiseAnnotation: "true",
					},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Behavior: &v2.HorizontalPodAutoscalerBehavior{
						ScaleDown: &v2.HPAScalingRules{
							Policies: []v2.HPAScalingPolicy{
								{
									Type:          v2.PercentScalingPolicy,
									Value:         2,
									PeriodSeconds: 90,
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
			c := New(fake.NewClientBuilder().Build(), record.NewFakeRecorder(10), 0.95, 90)
			if tt.initialHPA != nil {
				c = New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90)
			}
			_, err := c.InitializeHPA(context.Background(), tt.args.tortoise, tt.args.dm)
			if (err != nil) != tt.wantErr {
				t.Errorf("Service.InitializeHPA() error = %v, wantErr %v", err, tt.wantErr)
			}
			hpa := &v2.HorizontalPodAutoscaler{}
			err = c.c.Get(context.Background(), client.ObjectKey{Name: tt.afterHPA.Name, Namespace: tt.afterHPA.Namespace}, hpa)
			if err != nil {
				t.Errorf("get hpa error = %v", err)
			}

			if d := cmp.Diff(tt.afterHPA, hpa, cmpopts.IgnoreFields(v2.HorizontalPodAutoscaler{}, "TypeMeta"), cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")); d != "" {
				t.Errorf("Service.InitializeHPA() hpa diff = %v", d)
			}
		})
	}
}
