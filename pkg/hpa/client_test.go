package hpa

import (
	"context"
	"testing"
	"time"

	"k8s.io/utils/pointer"

	"github.com/google/go-cmp/cmp"

	"github.com/sanposhiho/tortoise/pkg/annotation"
	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/sanposhiho/tortoise/api/v1alpha1"
	v2 "k8s.io/api/autoscaling/v2"
)

func TestClient_UpdateHPAFromTortoiseRecommendation(t *testing.T) {
	now := metav1.Now()

	type args struct {
		ctx      context.Context
		tortoise *autoscalingv1alpha1.Tortoise
		now      time.Time
	}
	tests := []struct {
		name       string
		args       args
		initialHPA *v2.HorizontalPodAutoscaler
		want       *v2.HorizontalPodAutoscaler
		wantErr    bool
	}{
		{
			name: "Basic test case with external metrics",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1alpha1.Tortoise{
					Status: autoscalingv1alpha1.TortoiseStatus{
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: autoscalingv1alpha1.HorizontalRecommendations{
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
										To:        1,
										Value:     6,
										UpdatedAt: now,
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										Value:     3,
										UpdatedAt: now,
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
					Status: autoscalingv1alpha1.TortoiseStatus{
						Recommendations: autoscalingv1alpha1.Recommendations{
							Horizontal: autoscalingv1alpha1.HorizontalRecommendations{
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
										To:        1,
										Value:     6,
										UpdatedAt: now,
									},
								},
								MinReplicas: []autoscalingv1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										Value:     3,
										UpdatedAt: now,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{
				c: fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(),
			}
			got, err := c.UpdateHPAFromTortoiseRecommendation(tt.args.ctx, tt.args.tortoise, tt.args.now)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateHPAFromTortoiseRecommendation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if d := cmp.Diff(tt.want.Spec, got.Spec); d != "" {
				t.Errorf("UpdateHPAFromTortoiseRecommendation() diff = %v", d)
			}
		})
	}
}

func ptrInt32(i int32) *int32 {
	return &i
}

func resourceQuantityPtr(quantity resource.Quantity) *resource.Quantity {
	return &quantity
}
