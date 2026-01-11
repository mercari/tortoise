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
	"k8s.io/utils/ptr"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/features"
	"github.com/mercari/tortoise/pkg/utils"
)

func TestUpdateRecommendation(t *testing.T) {
	type args struct {
		tortoise          *v1beta3.Tortoise
		hpa               *v2.HorizontalPodAutoscaler
		currentReplicaNum int32
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
										AverageUtilization: ptr.To[int32](60),
									},
									Container: "app",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
				currentReplicaNum: 100,
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
			name: "when HPA hits the maximumMaxReplica, no recommendation is made",
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
						MaxReplicas: 1000, // Hit the maximumMaxReplica
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
										AverageUtilization: ptr.To[int32](60),
									},
									Container: "app",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
				currentReplicaNum: 1000, // Hit the maximumMaxReplica
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
					// No recommendation is made.
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
			name: "Hit minimumTargetResourceUtilization",
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
											Quantity: resource.MustParse("90Gi"),
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
										corev1.ResourceMemory: resource.MustParse("100Gi"),
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
										AverageUtilization: ptr.To[int32](20),
									},
									Container: "app",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
				currentReplicaNum: 100,
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
										corev1.ResourceMemory: 40, // minimumTargetResourceUtilization
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
									corev1.ResourceMemory: resource.MustParse("100Gi"),
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
										Quantity: resource.MustParse("90Gi"),
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
										AverageUtilization: ptr.To[int32](50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
				currentReplicaNum: 100,
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
										AverageUtilization: ptr.To[int32](50),
									},
									Container: "istio-proxy",
								},
							},
						},
					},
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
				currentReplicaNum: 100,
			},
			wantErr: false,
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
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
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("4"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("4Gi"),
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("0.6"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("644245094400m"),
									},
								},
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("5Gi"),
								},
							},
							{
								ContainerName: "istio-proxy",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
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
			s := New(2.0, 0.5, 90, 40, 3, 30, "50m", "50Mi", map[string]string{"istio-proxy": "100m"}, map[string]string{"istio-proxy": "100m"}, "10", "10Gi", 1000, 0.5, 0, nil, record.NewFakeRecorder(10))
			got, err := s.updateHPATargetUtilizationRecommendations(context.Background(), tt.args.tortoise, tt.args.hpa, tt.args.currentReplicaNum)
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
			name: "replica recommendation is replaced",
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
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     9,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
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
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     20,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "replica recommendation is not replaced",
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
										Value:     8,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     30,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
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
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     28,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "replica recommendation is not replaced and time-bias isn't applied",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)), // recently updated.
										TimeZone:  timeZone,
										Value:     8,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)), // recently updated.
										TimeZone:  timeZone,
										Value:     30,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
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
									Value:     8,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     30,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "hit minimumMinReplicas",
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
										Value:     0,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     0,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
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
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     12,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
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
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     3,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     1,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
								},
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     9,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      2,
										To:        3,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
									{
										From:      3,
										To:        4,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										TimeZone:  timeZone,
										Value:     7,
										WeekDay:   ptr.To(time.Sunday.String()),
									},
								},
							},
						},
					},
				},
				replicaNum: 10,
				now:        time.Date(2023, 3, 19, 5, 0, 0, 0, jst),
			},
			want: &v1beta3.Tortoise{
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
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     1,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     9,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      2,
									To:        3,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
								{
									From:      3,
									To:        4,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									TimeZone:  timeZone,
									Value:     7,
									WeekDay:   ptr.To(time.Sunday.String()),
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(2.0, 0.5, 90, 40, 3, 30, "50m", "50Mi", map[string]string{"istio-proxy": "100m"}, map[string]string{"istio-proxy": "100m"}, "10", "10Gi", 1000, 0.5, 0, nil, record.NewFakeRecorder(10))
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
	now := time.Now()
	type fields struct {
		preferredMaxReplicas          int32
		minimumMinReplicas            int32
		maxCPU                        string
		maxMemory                     string
		bufferRatioOnVerticalResource float64
		maxAllowedScalingDownRatio    float64
		features                      []features.FeatureFlag
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
			name: "all horizontal: replica count above preferredMaxReplicas: increase the resources a bit",
			fields: fields{
				preferredMaxReplicas: 3,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				features:             []features.FeatureFlag{features.VerticalScalingBasedOnPreferredMaxReplicas},
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddTortoiseConditions(v1beta3.TortoiseCondition{
					Type:    v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
					Status:  corev1.ConditionTrue,
					Reason:  "ScaledUpBasedOnPreferredMaxReplicas",
					Message: "the current number of replicas is bigger than the preferred max replica number",
					// not recently updated.
					LastTransitionTime: metav1.NewTime(now.Add(-31 * time.Minute)),
					LastUpdateTime:     metav1.NewTime(now.Add(-31 * time.Minute)),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("500Mi"),
						},
					},
				},
			).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionTrue,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("500m", "500Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("650m", "650Mi"), // current * 1.1
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: the temporal replica count being above preferredMaxReplicas doesn't trigger the resource increase of VerticalScalingBasedOnPreferredMaxReplicas",
			fields: fields{
				preferredMaxReplicas: 3,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				features:             []features.FeatureFlag{features.VerticalScalingBasedOnPreferredMaxReplicas},
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
						},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("500Mi"),
						},
					},
				},
			).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionTrue,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("500m", "500Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("500m", "500Mi"), // current
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: replica count above preferredMaxReplicas, but we recently increase the resource: don't increase the resources",
			fields: fields{
				preferredMaxReplicas: 3,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				features:             []features.FeatureFlag{features.VerticalScalingBasedOnPreferredMaxReplicas},
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
						},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddTortoiseConditions(v1beta3.TortoiseCondition{
					Type:    v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
					Status:  corev1.ConditionTrue,
					Reason:  "ScaledUpBasedOnPreferredMaxReplicas",
					Message: "the current number of replicas is bigger than the preferred max replica number",
					// recently updated.
					LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
					LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("400m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("400Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("500m", "500Mi"),
				}).Build(),
				replicaNum: 4,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("400m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("400Mi"),
						},
					},
				},
			).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:    v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:  corev1.ConditionTrue,
				Reason:  "ScaledUpBasedOnPreferredMaxReplicas",
				Message: "the current number of replicas is bigger than the preferred max replica number",
				// recently updated.
				LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
				LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
			}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("500m", "500Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("500m", "500Mi"), // Unchange
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: replica count above preferredMaxReplicas but VerticalScalingBasedOnPreferredMaxReplicas is disabled: increase the resources a bit",
			fields: fields{
				preferredMaxReplicas: 3,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				features:             []features.FeatureFlag{},
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
						},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("400m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("400Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("500m", "500Mi"),
				}).Build(),
				replicaNum: 4,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("400m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("400Mi"),
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
							RecommendedResource: createResourceList("500m", "500Mi"), // Unchange
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: replica count above preferredMaxReplicas: don't increase the resources when reaching minReplica",
			fields: fields{
				preferredMaxReplicas: 3,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				features:             []features.FeatureFlag{features.VerticalScalingBasedOnPreferredMaxReplicas},
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](4),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
						},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("400m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("400Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("500m", "500Mi"),
				}).Build(),
				replicaNum: 4,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("400m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("400Mi"),
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
							RecommendedResource: createResourceList("500m", "500Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "horizontal/vertical: replica count above preferredMaxReplicas: increase the resource a bit",
			fields: fields{
				preferredMaxReplicas: 3,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				features:             []features.FeatureFlag{features.VerticalScalingBasedOnPreferredMaxReplicas},
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddTortoiseConditions(v1beta3.TortoiseCondition{
					Type:    v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
					Status:  corev1.ConditionTrue,
					Reason:  "ScaledUpBasedOnPreferredMaxReplicas",
					Message: "the current number of replicas is bigger than the preferred max replica number",
					// not recently updated.
					LastTransitionTime: metav1.NewTime(now.Add(-31 * time.Minute)),
					LastUpdateTime:     metav1.NewTime(now.Add(-31 * time.Minute)),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("500m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("800Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("500m", "500Mi"),
				}).Build(),
				replicaNum: 4,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("800Mi"),
						},
					},
				},
			).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionTrue,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("500m", "500Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("650m" /* current * 1.1*/, "800Mi" /* VPA recommendation*/),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: recommendation exceed the global maxResourceSize",
			fields: fields{
				preferredMaxReplicas: 5,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("1500m"), // exceed maxResourceSize
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("1.5Gi"), // exceed maxResourceSize
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("1500m", "1.5Gi"), // exceed maxResourceSize
				}).Build(),
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](20),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](20),
									},
									Container: "test-container",
								},
							},
						},
					},
				},
				replicaNum: 4,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("1500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("1.5Gi"),
						},
					},
				},
			).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionFalse,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is not bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("1500m", "1.5Gi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("1000m", "1Gi"), // maxResourceSize
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: requested resources is smaller than MinAllocatedResources",
			fields: fields{
				preferredMaxReplicas: 5,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("15m"), // smaller than MinAllocatedResources
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("1.5Mi"), //smaller than MinAllocatedResources
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("15m", "1.5Mi"), //smaller than MinAllocatedResources
				}).Build(),
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](20),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](20),
									},
									Container: "test-container",
								},
							},
						},
					},
				},
				replicaNum: 4,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("15m"), // smaller than MinAllocatedResources
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("1.5Mi"), //smaller than MinAllocatedResources
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("15m", "1.5Mi"), //smaller than MinAllocatedResources
			}).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionFalse,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is not bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("100m", "100Mi"), // minAllocatedResources
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: replica number is too small: reduce resources based on VPA recommendation",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
				minimumMinReplicas:   3,
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
				replicaNum: 3, // Too small
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			}).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionFalse,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is not bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
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
			name: "all vertical: reduce resources based on VPA recommendation",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			name: "all vertical: scale resources based on VPA recommendation plus bufferRatioOnVerticalResource",
			fields: fields{
				preferredMaxReplicas:          6,
				maxCPU:                        "1000m",
				maxMemory:                     "1Gi",
				bufferRatioOnVerticalResource: 0.1,
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
							ContainerName: "test-container",
							// VPA recommendation * (1 + bufferRatioOnVerticalResource) * (1 + bufferRatioOnVerticalResource)
							RecommendedResource: createResourceList("145m", "145.2Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: scale down resources based on VPA recommendation plus bufferRatioOnVerticalResource",
			fields: fields{
				preferredMaxReplicas:          6,
				maxCPU:                        "1000m",
				maxMemory:                     "1Gi",
				bufferRatioOnVerticalResource: 0.1,
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								// ideal = 110m
								Quantity: resource.MustParse("100m"),
							},
							corev1.ResourceMemory: {
								// ideal = 121Mi
								Quantity: resource.MustParse("110Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					// Previous ideal = 132m, 132Mi
					// Previous ideal * 0.9 = 118m, 118.8Mi
					//
					// → CPU will be scaled down to 110m
					// → Memory will not be scaled down (because 118.8Mi < 121Mi)
					Resource: createResourceList("145m", "145.2Mi"),
				}).SetRecommendations(v1beta3.Recommendations{
					Vertical: v1beta3.VerticalRecommendations{
						ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
							{
								ContainerName:       "test-container",
								RecommendedResource: createResourceList("145m", "145.2Mi"),
							},
						},
					},
				}).Build(),
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("100m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("110Mi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "test-container",
				Resource:      createResourceList("145m", "145.2Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName: "test-container",
							// VPA recommendation * (1 + bufferRatioOnVerticalResource) * (1 + bufferRatioOnVerticalResource)
							RecommendedResource: createResourceList("110m", "145.2Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: use MaxAllocatedResources when VPA recommendation is bigger than MaxAllocatedResources",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MaxAllocatedResources: createResourceList("300m", "300Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("500m"), // too big
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("0.5Gi"), // too big
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MaxAllocatedResources: createResourceList("300m", "300Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("0.5Gi"),
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
							RecommendedResource: createResourceList("300m", "300Mi"), // same as MinAllocatedResources
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: use global MaxAllocatedResources when defined MaxAllocatedResources is bigger than global MaxAllocatedResources",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "300m",
				maxMemory:            "300Mi",
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MaxAllocatedResources: createResourceList("1000m", "1Gi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("500m"), // too big
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("0.5Gi"), // too big
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MaxAllocatedResources: createResourceList("1000m", "1Gi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("500m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("0.5Gi"),
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
							RecommendedResource: createResourceList("300m", "300Mi"), // same as MinAllocatedResources
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: use MinAllocatedResources when VPA recommendation is smaller than MinAllocatedResources",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("10m"), // too small
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("10Mi"), // too small
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("100m", "100Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("10m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("10Mi"),
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
							RecommendedResource: createResourceList("100m", "100Mi"), // same as MinAllocatedResources
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: use minResourceSize when VPA recommendation is smaller than minResourceSize",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "test-container",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "test-container",
					MinAllocatedResources: createResourceList("1m", "1Mi"), // They're overwriteen by minResourceSize because minResourceSize is bigger than them.
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("2m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("2Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "test-container",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "test-container",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "test-container",
				MinAllocatedResources: createResourceList("1m", "1Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("2m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("2Mi"),
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
							RecommendedResource: createResourceList("5m", "5Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: use minResourceSize (per container) when VPA recommendation is smaller than minResourceSize",
			fields: fields{
				preferredMaxReplicas: 6,
				maxCPU:               "1000m",
				maxMemory:            "1Gi",
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "istio-proxy",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
					ContainerName:         "istio-proxy",
					MinAllocatedResources: createResourceList("1m", "1Mi"), // They're overwriteen by minResourceSize because minResourceSize is bigger than them.
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "istio-proxy",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("2m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("2Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "istio-proxy",
					Resource:      createResourceList("130m", "130Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "istio-proxy",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddResourcePolicy(v1beta3.ContainerResourcePolicy{
				ContainerName:         "istio-proxy",
				MinAllocatedResources: createResourceList("1m", "1Mi"),
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "istio-proxy",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("2m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("2Mi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "istio-proxy",
				Resource:      createResourceList("130m", "130Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "istio-proxy",
							RecommendedResource: createResourceList("7m", "7Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all vertical: use oldSizeMilli * s.maxAllowedScalingDownRatio when VPA recommendation is smaller than oldSizeMilli * s.maxAllowedScalingDownRatio",
			fields: fields{
				preferredMaxReplicas:       6,
				maxCPU:                     "1000m",
				maxMemory:                  "1Gi",
				maxAllowedScalingDownRatio: 0.8, // At maximum, it can be reduced to 0.8*current req.
			},
			args: args{
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics:     []v2.MetricSpec{},
					},
				},
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
					ContainerName: "istio-proxy",
					Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
						corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "istio-proxy",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							// It's too small. They're overwriteen based on maxAllowedScalingDownRatio.
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("2m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("2Mi"),
							},
						},
					},
				).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
					ContainerName: "istio-proxy",
					Resource:      createResourceList("100m", "100Mi"),
				}).Build(),
				replicaNum: 3,
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
				ContainerName: "istio-proxy",
				Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
					corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
					corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
				},
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "istio-proxy",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("2m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("2Mi"),
						},
					},
				},
			).AddContainerResourceRequests(v1beta3.ContainerResourceRequests{
				ContainerName: "istio-proxy",
				Resource:      createResourceList("100m", "100Mi"),
			}).SetRecommendations(v1beta3.Recommendations{
				Vertical: v1beta3.VerticalRecommendations{
					ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
						{
							ContainerName:       "istio-proxy",
							RecommendedResource: createResourceList("80m", "80Mi"),
						},
					},
				},
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: reduced resources based on VPA recommendation when unbalanced container size in multiple containers Pod",
			fields: fields{
				preferredMaxReplicas: 10,
				maxCPU:               "10000m",
				maxMemory:            "100Gi",
			},
			args: args{
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
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
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("80m"), // smaller than expectation (800m+)
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("9Gi"),
							},
						},
					},
				).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container2",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("800m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("0.7Gi"), // smaller than expectation (7Gi+)
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
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](70),
									},
									Container: "test-container2",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](70),
									},
									Container: "test-container2",
								},
							},
						},
					},
				},
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
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
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
			}).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionFalse,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is not bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).Build(),
			wantErr: false,
		},
		{
			name: "all horizontal: no scale down happens if replica num is close to preferredMaxReplicas, even when unbalanced container size in multiple containers Pod",
			fields: fields{
				preferredMaxReplicas: 10,
				maxCPU:               "10000m",
				maxMemory:            "100Gi",
			},
			args: args{
				tortoise: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
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
				}).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("80m"), // smaller than expectation (800m+)
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("9Gi"),
							},
						},
					},
				).AddContainerRecommendationFromVPA(
					v1beta3.ContainerRecommendationFromVPA{
						ContainerName: "test-container2",
						MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("800m"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("0.7Gi"), // smaller than expectation (7Gi+)
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
				replicaNum: 9,
				hpa: &v2.HorizontalPodAutoscaler{
					Spec: v2.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						Metrics: []v2.MetricSpec{
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "test-container",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](70),
									},
									Container: "test-container2",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](70),
									},
									Container: "test-container2",
								},
							},
						},
					},
				},
			},
			want: utils.NewTortoiseBuilder().AddAutoscalingPolicy(v1beta3.ContainerAutoscalingPolicy{
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
			}).AddContainerRecommendationFromVPA(
				v1beta3.ContainerRecommendationFromVPA{
					ContainerName: "test-container",
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
					MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
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
						// no scale down.
						{
							ContainerName:       "test-container",
							RecommendedResource: createResourceList("1000m", "10Gi"),
						},
						{
							ContainerName:       "test-container2",
							RecommendedResource: createResourceList("1000m", "10Gi"),
						},
					},
				},
			}).AddTortoiseConditions(v1beta3.TortoiseCondition{
				Type:               v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas,
				Status:             corev1.ConditionFalse,
				Reason:             "ScaledUpBasedOnPreferredMaxReplicas",
				Message:            "the current number of replicas is not bigger than the preferred max replica number",
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
			}).Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			s := New(0, 0, 0, 0, int(tt.fields.minimumMinReplicas), int(tt.fields.preferredMaxReplicas), "5m", "5Mi", map[string]string{"istio-proxy": "7m"}, map[string]string{"istio-proxy": "7Mi"}, tt.fields.maxCPU, tt.fields.maxMemory, 10000, tt.fields.maxAllowedScalingDownRatio, tt.fields.bufferRatioOnVerticalResource, tt.fields.features, record.NewFakeRecorder(10))
			got, err := s.updateVPARecommendation(context.Background(), tt.args.tortoise, tt.args.hpa, tt.args.replicaNum, now)
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
