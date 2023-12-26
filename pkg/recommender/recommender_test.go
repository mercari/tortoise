package recommender

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/utils"
)

func TestUpdateRecommendation(t *testing.T) {
	type args struct {
		tortoise *v1beta3.Tortoise
		hpa      *v2.HorizontalPodAutoscaler
	}
	tests := []struct {
		name    string
		args    args
		want    *v1beta3.Tortoise
		wantErr bool
	}{
		{
			name: "HPA has the container resource metrics",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
								{
									ContainerName: "app",
									MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
										corev1.ResourceMemory: {
											Quantity: resource.MustParse("4Gi"),
										},
										corev1.ResourceCPU: {
											Quantity: resource.MustParse("4"),
										},
									},
								},
								{
									ContainerName: "istio-proxy",
									MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
										corev1.ResourceMemory: {
											Quantity: resource.MustParse("0.6Gi"),
										},
										corev1.ResourceCPU: {
											Quantity: resource.MustParse("0.6"),
										},
									},
								},
							},
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("5Gi"),
										corev1.ResourceCPU:    resource.MustParse("5"),
									},
								},
								{
									ContainerName: "istio-proxy",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						Metrics: []v2.MetricSpec{
							{
								// unrelated
								Type: v2.ObjectMetricSourceType,
							},
							{
								// unrelated
								Type: v2.ExternalMetricSourceType,
								External: &v2.ExternalMetricSource{
									Metric: v2.MetricIdentifier{
										Name: "datadogmetric@echo-prod:echo-cpu-istio-proxy",
									},
									Target: v2.MetricTarget{
										Value: resourceQuantityPtr(resource.MustParse("90")),
									},
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(60),
									},
									Container: "app",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
								{
									ContainerName: "app",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceMemory: 80,
									},
								},
								{
									ContainerName: "istio-proxy",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceCPU: 90,
									},
								},
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("5Gi"),
									corev1.ResourceCPU:    resource.MustParse("5"),
								},
							},
							{
								ContainerName: "istio-proxy",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("4Gi"),
									},
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("4"),
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("0.6Gi"),
									},
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("0.6"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Tortoise has some AutoscalingTypeOff policy",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
								{
									ContainerName: "app",
									MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
										corev1.ResourceMemory: {
											Quantity: resource.MustParse("4Gi"),
										},
										corev1.ResourceCPU: {
											Quantity: resource.MustParse("4"),
										},
									},
								},
								{
									ContainerName: "istio-proxy",
									MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
										corev1.ResourceMemory: {
											Quantity: resource.MustParse("0.6Gi"),
										},
										corev1.ResourceCPU: {
											Quantity: resource.MustParse("0.6"),
										},
									},
								},
							},
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("5Gi"),
										corev1.ResourceCPU:    resource.MustParse("5"),
									},
								},
								{
									ContainerName: "istio-proxy",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						Metrics: []v2.MetricSpec{
							{
								// unrelated
								Type: v2.ObjectMetricSourceType,
							},
							{
								// unrelated
								Type: v2.ExternalMetricSourceType,
								External: &v2.ExternalMetricSource{
									Metric: v2.MetricIdentifier{
										Name: "datadogmetric@echo-prod:echo-cpu-istio-proxy",
									},
									Target: v2.MetricTarget{
										Value: resourceQuantityPtr(resource.MustParse("90")),
									},
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
								{
									ContainerName:     "app",
									TargetUtilization: map[corev1.ResourceName]int32{},
								},
								{
									ContainerName: "istio-proxy",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceCPU: 90,
									},
								},
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("5Gi"),
									corev1.ResourceCPU:    resource.MustParse("5"),
								},
							},
							{
								ContainerName: "istio-proxy",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("4Gi"),
									},
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("4"),
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("0.6Gi"),
									},
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("0.6"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "HPA should have the container resource metrics, but doesn't",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
								{
									ContainerName: "app",
									MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
										corev1.ResourceMemory: {
											Quantity: resource.MustParse("4Gi"),
										},
										corev1.ResourceCPU: {
											Quantity: resource.MustParse("4"),
										},
									},
								},
								{
									ContainerName: "istio-proxy",
									MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
										corev1.ResourceMemory: {
											Quantity: resource.MustParse("0.6Gi"),
										},
										corev1.ResourceCPU: {
											Quantity: resource.MustParse("0.6"),
										},
									},
								},
							},
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("5Gi"),
										corev1.ResourceCPU:    resource.MustParse("5"),
									},
								},
								{
									ContainerName: "istio-proxy",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						Metrics: []v2.MetricSpec{
							{
								// unrelated
								Type: v2.ObjectMetricSourceType,
							},
							{
								// unrelated
								Type: v2.ExternalMetricSourceType,
								External: &v2.ExternalMetricSource{
									Metric: v2.MetricIdentifier{
										Name: "datadogmetric@echo-prod:echo-cpu-istio-proxy",
									},
									Target: v2.MetricTarget{
										Value: resourceQuantityPtr(resource.MustParse("90")),
									},
								},
							},
							// the container metric for "app" container is missing.
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(24*30, 2.0, 0.5, 90, 3, 30, "10", "10Gi", record.NewFakeRecorder(10))
			got, err := s.updateHPATargetUtilizationRecommendations(context.Background(), tt.args.tortoise, tt.args.hpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateHPATargetUtilizationRecommendations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if d := cmp.Diff(tt.want, got); d != "" {
				t.Errorf("unexpected result from updateHPARecommendation; diff = %s", d)
			}
		})
	}
}

func resourceQuantityPtr(quantity resource.Quantity) *resource.Quantity {
	return &quantity
}

func Test_updateHPAMinMaxReplicasRecommendations(t *testing.T) {
	timeZone := "Asia/Tokyo"
	jst, err := time.LoadLocation(timeZone)
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		tortoise   *v1beta3.Tortoise
		replicaNum int32
		now        time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    *v1beta3.Tortoise
		wantErr bool
	}{
		{
			name: "Basic case",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
							},
						},
					},
				},
				replicaNum: 10,
				now:        time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     5,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     20,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "No recommendation slot",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								MinReplicas: []v1beta3.ReplicasRecommendation{},
								MaxReplicas: []v1beta3.ReplicasRecommendation{},
							},
						},
					},
				},
				replicaNum: 5,
				now:        time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{},
							MaxReplicas: []v1beta3.ReplicasRecommendation{},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Lower recommendation value",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     10,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     25,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
							},
						},
					},
				},
				replicaNum: 5,
				now:        time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From: 0,
									To:   1,
									// UpdatedAt is updated.
									TimeZone:  timeZone,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									Value:     10,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From: 0,
									To:   1,
									// UpdatedAt is updated.
									TimeZone:  timeZone,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									Value:     25,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Same recommendation value",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     3,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     8,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
							},
						},
					},
				},
				replicaNum: 10,
				now:        time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     5,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     20,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "minimum MinReplicas and minimum MaxReplicas",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     3,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     8,
										WeekDay:   pointer.String(time.Sunday.String()),
									},
								},
							},
						},
					},
				},
				replicaNum: 1,
				now:        time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     3,
									WeekDay:   pointer.String(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     12,
									WeekDay:   pointer.String(time.Sunday.String()),
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
			s := New(24*30, 2.0, 0.5, 90, 3, 30, "10", "10Gi", record.NewFakeRecorder(10))
			got, err := s.updateHPAMinMaxReplicasRecommendations(tt.args.tortoise, tt.args.replicaNum, tt.args.now)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateHPAMinMaxReplicasRecommendations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if d := cmp.Diff(got, tt.want); d != "" {
				t.Errorf("updateHPAMinMaxReplicasRecommendations() diff = %v", d)
			}
		})
	}
}

func TestService_UpdateVPARecommendation(t *testing.T) {
	type fields struct {
		preferredReplicaNumUpperLimit int32
		minimumMinReplicas            int32
		suggestedResourceSizeAtMax    corev1.ResourceList
	}
	type args struct {
		tortoise   *v1beta3.Tortoise
		hpa        *v2.HorizontalPodAutoscaler
		replicaNum int32
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *v1beta3.Tortoise
		wantErr bool
	}{
		{
			name: "replica count above preferredReplicaNumUpperLimit",
			fields: fields{
				preferredReplicaNumUpperLimit: 3,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
			},
			args: args{
				tortoise: createTortoise().AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("500m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("500Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("500m", "500Mi"),
				}).Build(),
				replicaNum: 4,
			},
			want: createTortoise().AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("500Mi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("500m", "500Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("550m", "550Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "requested resources exceed maxResourceSize",
			fields: fields{
				preferredReplicaNumUpperLimit: 3,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
			},
			args: args{
				tortoise: createTortoise().AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("1500m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("1.5Gi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("1500m", "1.5Gi"),
				}).Build(),
				replicaNum: 4,
			},
			want: createTortoise().AddContainerRecommendationFromVPA(
				// Unchanged!
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("1500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("1.5Gi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("1500m", "1.5Gi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("1500m", "1.5Gi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "reduced resources based on VPA recommendation",
			fields: fields{
				preferredReplicaNumUpperLimit: 6,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
				minimumMinReplicas:            3,
			},
			args: args{
				tortoise: createTortoise().AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("120m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("120Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: createTortoise().AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("120m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("120Mi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("130m", "130Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("120m", "120Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "vertical only: reduced resources based on VPA recommendation",
			fields: fields{
				preferredReplicaNumUpperLimit: 6,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
			},
			args: args{
				tortoise: createVerticalTortoise().AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("120m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("120Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: createVerticalTortoise().AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("120m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("120Mi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("130m", "130Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("120m", "120Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "reduced resources based on VPA recommendation when unbalanced container size in multiple containers Pod",
			fields: fields{
				preferredReplicaNumUpperLimit: 6,
				suggestedResourceSizeAtMax:    createResourceList("10000m", "100Gi"),
			},
			args: args{
				tortoise: createTortoiseWithMultipleContainers().AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("80m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("9Gi"),
							},
						},
					},
				).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container2",
						Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("800m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("0.7Gi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("1000m", "10Gi"),
				}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container2",
					Resource:      createResourceList("1000m", "10Gi"),
				}).Build(),
				replicaNum: 5,
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(70),
									},
									Container: "test-container2",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: pointer.Int32(70),
									},
									Container: "test-container2",
								},
							},
						},
					},
				},
			},
			want: createTortoiseWithMultipleContainers().AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("80m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("9Gi"),
						},
					},
				},
			).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container2",
					Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("800m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("0.7Gi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("1000m", "10Gi"),
			}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container2",
				Resource:      createResourceList("1000m", "10Gi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("100m", "10Gi"),
						},
						{
							ContainerName:       "test-container2",
							RecommendedResource: createResourceList("1000m", "1Gi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				minimumMinReplicas:            tt.fields.minimumMinReplicas,
				preferredReplicaNumUpperLimit: tt.fields.preferredReplicaNumUpperLimit,
				maxResourceSize:               tt.fields.suggestedResourceSizeAtMax,
				eventRecorder:                 record.NewFakeRecorder(10),
			}
			got, err := s.updateVPARecommendation(context.Background(), tt.args.tortoise, tt.args.hpa, tt.args.replicaNum)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateVPARecommendation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if d := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
				t.Errorf("updateVPARecommendation() diff = %s", d)
			}
		})
	}
}

// Helper functions to create test objects
func createResourceList(cpu, memory string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(memory),
	}
}
func createVerticalTortoise() *utils.TortoiseBuilder {
	return utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
		ContainerName: "test-container",
		Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
			corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
			corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
		},
	}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
		ContainerName:         "test-container",
		MinAllocatedResources: createResourceList("100m", "100Mi"),
	})
}
func createTortoise() *utils.TortoiseBuilder {
	b := utils.NewTortoiseBuilder()
	// Build the above tortoise via builder
	b.AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
		ContainerName: "test-container",
		Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
			corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
			corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
		},
	}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
		ContainerName:         "test-container",
		MinAllocatedResources: createResourceList("100m", "100Mi"),
	})
	return b
}
func createTortoiseWithMultipleContainers() *utils.TortoiseBuilder {
	return utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
		ContainerName: "test-container",
		Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
			corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
			corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
		},
	}).AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
		ContainerName: "test-container2",
		Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
			corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
			corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
		},
	}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
		ContainerName:         "test-container2",
		MinAllocatedResources: createResourceList("100m", "100Mi"),
	}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
		ContainerName:         "test-container",
		MinAllocatedResources: createResourceList("100m", "100Mi"),
	})
}
