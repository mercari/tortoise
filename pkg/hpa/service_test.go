package hpa

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	"github.com/mercari/tortoise/pkg/annotation"
)

func TestClient_UpdateHPAFromTortoiseRecommendation(t *testing.T) {
	now := metav1.NewTime(time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC))

	type args struct {
		ctx      context.Context
		tortoise *autoscalingv1alpha1.Tortoise
		now      time.Time
	}
	tests := []struct {
		name         string
		args         args
		initialHPA   *v2.HorizontalPodAutoscaler
		want         *v2.HorizontalPodAutoscaler
		wantTortoise *autoscalingv1alpha1.Tortoise
		wantErr      bool
	}{
		{
			name: "Basic test case with external metrics",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1alpha1.Tortoise{
					Spec: autoscalingv1alpha1.TortoiseSpec{
						TargetRefs: autoscalingv1alpha1.TargetRefs{
							HorizontalPodAutoscalerName: pointer.String("hpa"),
						},
					},
					Status: autoscalingv1alpha1.TortoiseStatus{
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: &autoscalingv1alpha1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1alpha1.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "datadogmetric@echo-prod:echo-memory-app",
								},
								Target: v2.MetricTarget{
									Value: resourceQuantityPtr(resource.MustParse("60")),
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "datadogmetric@echo-prod:echo-cpu-istio-proxy",
								},
								Target: v2.MetricTarget{
									Value: resourceQuantityPtr(resource.MustParse("50")),
								},
							},
						},
					},
				},
			},
			want: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "datadogmetric@echo-prod:echo-memory-app",
								},
								Target: v2.MetricTarget{
									Value: resourceQuantityPtr(resource.MustParse("90")),
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "datadogmetric@echo-prod:echo-cpu-istio-proxy",
								},
								Target: v2.MetricTarget{
									Value: resourceQuantityPtr(resource.MustParse("80")),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Basic test case with container resource metrics",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1alpha1.Tortoise{
					Spec: autoscalingv1alpha1.TortoiseSpec{
						TargetRefs: autoscalingv1alpha1.TargetRefs{
							HorizontalPodAutoscalerName: pointer.String("hpa"),
						},
					},
					Status: autoscalingv1alpha1.TortoiseStatus{
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: &autoscalingv1alpha1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1alpha1.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
			name: "emergency mode",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1alpha1.Tortoise{
					Spec: autoscalingv1alpha1.TortoiseSpec{
						TargetRefs: autoscalingv1alpha1.TargetRefs{
							HorizontalPodAutoscalerName: pointer.String("hpa"),
						},
					},
					Status: autoscalingv1alpha1.TortoiseStatus{
						TortoisePhase: autoscalingv1alpha1.TortoisePhaseEmergency,
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: &autoscalingv1alpha1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1alpha1.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
				tortoise: &autoscalingv1alpha1.Tortoise{
					Spec: autoscalingv1alpha1.TortoiseSpec{
						TargetRefs: autoscalingv1alpha1.TargetRefs{
							HorizontalPodAutoscalerName: pointer.String("hpa"),
						},
					},
					Status: autoscalingv1alpha1.TortoiseStatus{
						TortoisePhase: autoscalingv1alpha1.TortoisePhaseBackToNormal,
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: &autoscalingv1alpha1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1alpha1.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
				tortoise: &autoscalingv1alpha1.Tortoise{
					Spec: autoscalingv1alpha1.TortoiseSpec{
						TargetRefs: autoscalingv1alpha1.TargetRefs{
							HorizontalPodAutoscalerName: pointer.String("hpa"),
						},
					},
					Status: autoscalingv1alpha1.TortoiseStatus{
						TortoisePhase: autoscalingv1alpha1.TortoisePhaseBackToNormal,
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: &autoscalingv1alpha1.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1alpha1.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   now.Weekday().String(),
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
					Annotations: map[string]string{
						annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
						annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
					},
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
			wantTortoise: &autoscalingv1alpha1.Tortoise{
				Spec: autoscalingv1alpha1.TortoiseSpec{
					TargetRefs: autoscalingv1alpha1.TargetRefs{
						HorizontalPodAutoscalerName: pointer.String("hpa"),
					},
				},
				Status: autoscalingv1alpha1.TortoiseStatus{
					TortoisePhase: autoscalingv1alpha1.TortoisePhaseWorking,
					Recommendations: autoscalingv1alpha1.Recommendations{
						Horizontal: &autoscalingv1alpha1.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1alpha1.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   now.Weekday().String(),
								},
							},
							MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   now.Weekday().String(),
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
			c := New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), 0.95, 90)
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
