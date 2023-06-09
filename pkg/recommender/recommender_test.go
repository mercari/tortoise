package recommender

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/mercari/tortoise/api/v1alpha1"
	"github.com/mercari/tortoise/pkg/annotation"
)

func TestUpdateRecommendation(t *testing.T) {
	type args struct {
		tortoise   *v1alpha1.Tortoise
		hpa        *v2.HorizontalPodAutoscaler
		deployment *v1.Deployment
	}
	tests := []struct {
		name string
		args args
		want *v1alpha1.Tortoise
	}{
		{
			name: "if HPA has the container resource metrics, then it has higher priority than external metrics",
			args: args{
				tortoise: &v1alpha1.Tortoise{
					Spec: v1alpha1.TortoiseSpec{
						ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
							{
								ContainerName: "app",
								AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
									corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
									corev1.ResourceCPU:    v1alpha1.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
									corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
									corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								},
							},
						},
					},
					Status: v1alpha1.TortoiseStatus{
						Conditions: v1alpha1.Conditions{
							ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
								{
									ContainerName: "app",
									MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
									MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
				hpa: &v2.HorizontalPodAutoscaler{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							// they shouldn't affect.
							annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
							annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
						},
					},
					Spec: v2.HorizontalPodAutoscalerSpec{
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
				deployment: &v1.Deployment{
					Spec: v1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "app",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("5Gi"),
												corev1.ResourceCPU:    resource.MustParse("5"),
											},
										},
									},
									{
										Name: "istio-proxy",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("1Gi"),
												corev1.ResourceCPU:    resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: &v1alpha1.Tortoise{
				Spec: v1alpha1.TortoiseSpec{
					ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "istio-proxy",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1alpha1.TortoiseStatus{
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							TargetUtilizations: []v1alpha1.HPATargetUtilizationRecommendationPerContainer{
								{
									ContainerName: "app",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceCPU:    90,
										corev1.ResourceMemory: 80,
									},
								},
								{
									ContainerName: "istio-proxy",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceCPU:    90,
										corev1.ResourceMemory: 90,
									},
								},
							},
						},
					},
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
			name: "if HPA doesn't have the container resource metrics, then the external metrics are used",
			args: args{
				tortoise: &v1alpha1.Tortoise{
					Spec: v1alpha1.TortoiseSpec{
						ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
							{
								ContainerName: "app",
								AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
									corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
									corev1.ResourceCPU:    v1alpha1.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
									corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
									corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								},
							},
						},
					},
					Status: v1alpha1.TortoiseStatus{
						Conditions: v1alpha1.Conditions{
							ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{

								{
									ContainerName: "app",
									MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
									MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
				hpa: &v2.HorizontalPodAutoscaler{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: "datadogmetric@echo-prod:echo-memory-",
							annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    "datadogmetric@echo-prod:echo-cpu-",
						},
					},
					Spec: v2.HorizontalPodAutoscalerSpec{
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
					Status: v2.HorizontalPodAutoscalerStatus{},
				},
				deployment: &v1.Deployment{
					Spec: v1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "app",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("5Gi"),
												corev1.ResourceCPU:    resource.MustParse("5"),
											},
										},
									},
									{
										Name: "istio-proxy",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("1Gi"),
												corev1.ResourceCPU:    resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: &v1alpha1.Tortoise{
				Spec: v1alpha1.TortoiseSpec{
					ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "istio-proxy",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1alpha1.TortoiseStatus{
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							TargetUtilizations: []v1alpha1.HPATargetUtilizationRecommendationPerContainer{
								{
									ContainerName: "app",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceMemory: 80,
										corev1.ResourceCPU:    90,
									},
								},
								{
									ContainerName: "istio-proxy",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceCPU:    90,
										corev1.ResourceMemory: 90,
									},
								},
							},
						},
					},
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(24*30, 2.0, 0.5, 90, 3, 30, "10", "10Gi")
			got, err := s.updateHPATargetUtilizationRecommendations(context.Background(), tt.args.tortoise, tt.args.hpa, tt.args.deployment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
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
		tortoise   *v1alpha1.Tortoise
		deployment *v1.Deployment
		now        time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    *v1alpha1.Tortoise
		wantErr bool
	}{
		{
			name: "Basic case",
			args: args{
				tortoise: &v1alpha1.Tortoise{
					Status: v1alpha1.TortoiseStatus{
						Recommendations: v1alpha1.Recommendations{
							Horizontal: &v1alpha1.HorizontalRecommendations{
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										Value:     1,
										WeekDay:   time.Sunday.String(),
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										Value:     7,
										WeekDay:   time.Sunday.String(),
									},
								},
							},
						},
					},
				},
				deployment: &v1.Deployment{
					Status: v1.DeploymentStatus{
						Replicas: 4,
					},
				},
				now: time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									Value:     3,
									WeekDay:   time.Sunday.String(),
								},
							},
							MaxReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									Value:     8,
									WeekDay:   time.Sunday.String(),
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
				tortoise: &v1alpha1.Tortoise{
					Status: v1alpha1.TortoiseStatus{
						Recommendations: v1alpha1.Recommendations{
							Horizontal: &v1alpha1.HorizontalRecommendations{
								MinReplicas: []v1alpha1.ReplicasRecommendation{},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{},
							},
						},
					},
				},
				deployment: &v1.Deployment{
					Status: v1.DeploymentStatus{
						Replicas: 5,
					},
				},
				now: time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							MinReplicas: []v1alpha1.ReplicasRecommendation{},
							MaxReplicas: []v1alpha1.ReplicasRecommendation{},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Lower recommendation value",
			args: args{
				tortoise: &v1alpha1.Tortoise{
					Status: v1alpha1.TortoiseStatus{
						Recommendations: v1alpha1.Recommendations{
							Horizontal: &v1alpha1.HorizontalRecommendations{
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										Value:     10,
										WeekDay:   time.Sunday.String(),
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										Value:     25,
										WeekDay:   time.Sunday.String(),
									},
								},
							},
						},
					},
				},
				deployment: &v1.Deployment{
					Status: v1.DeploymentStatus{
						Replicas: 5,
					},
				},
				now: time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From: 0,
									To:   1,
									// UpdatedAt is updated.
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									Value:     10,
									WeekDay:   time.Sunday.String(),
								},
							},
							MaxReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From: 0,
									To:   1,
									// UpdatedAt is updated.
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
									Value:     25,
									WeekDay:   time.Sunday.String(),
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
				tortoise: &v1alpha1.Tortoise{
					Status: v1alpha1.TortoiseStatus{
						Recommendations: v1alpha1.Recommendations{
							Horizontal: &v1alpha1.HorizontalRecommendations{
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										Value:     3,
										WeekDay:   time.Sunday.String(),
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        1,
										UpdatedAt: metav1.NewTime(time.Date(2023, 3, 12, 0, 0, 0, 0, jst)),
										Value:     8,
										WeekDay:   time.Sunday.String(),
									},
								},
							},
						},
					},
				},
				deployment: &v1.Deployment{
					Status: v1.DeploymentStatus{
						Replicas: 4,
					},
				},
				now: time.Date(2023, 3, 19, 0, 0, 0, 0, jst),
			},
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									Value:     3,
									WeekDay:   time.Sunday.String(),
								},
							},
							MaxReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        1,
									UpdatedAt: metav1.NewTime(time.Date(2023, 3, 19, 0, 0, 0, 0, jst)),
									Value:     8,
									WeekDay:   time.Sunday.String(),
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
			s := New(24*30, 2.0, 0.5, 90, 3, 30, "10", "10Gi")
			got, err := s.updateHPAMinMaxReplicasRecommendations(tt.args.tortoise, tt.args.deployment, tt.args.now)
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
		tortoise   *v1alpha1.Tortoise
		deployment *v1.Deployment
		hpa        *v2.HorizontalPodAutoscaler
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *v1alpha1.Tortoise
		wantErr bool
	}{
		{
			name: "replica count above preferredReplicaNumUpperLimit",
			fields: fields{
				preferredReplicaNumUpperLimit: 3,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
			},
			args: args{
				tortoise: createTortoiseWithCondition(map[corev1.ResourceName]v1alpha1.ResourceQuantity{
					corev1.ResourceCPU: {
						Quantity: resource.MustParse("500m"),
					},
					corev1.ResourceMemory: {
						Quantity: resource.MustParse("500Mi"),
					},
				}),
				deployment: createDeployment(4, "500m", "500Mi"),
			},
			want:    createTortoiseWithVPARecommendation("550m", "550Mi"),
			wantErr: false,
		},
		{
			name: "requested resources exceed maxResourceSize",
			fields: fields{
				preferredReplicaNumUpperLimit: 3,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
			},
			args: args{
				tortoise: createTortoiseWithCondition(map[corev1.ResourceName]v1alpha1.ResourceQuantity{
					corev1.ResourceCPU: {
						Quantity: resource.MustParse("1500m"),
					},
					corev1.ResourceMemory: {
						Quantity: resource.MustParse("1.5Gi"),
					},
				}),
				deployment: createDeployment(4, "1500m", "1.5Gi"),
			},
			want:    createTortoiseWithVPARecommendation("1500m", "1.5Gi"),
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
				tortoise: createTortoiseWithCondition(map[corev1.ResourceName]v1alpha1.ResourceQuantity{
					corev1.ResourceCPU: {
						Quantity: resource.MustParse("120m"),
					},
					corev1.ResourceMemory: {
						Quantity: resource.MustParse("120Mi"),
					},
				}),
				deployment: createDeployment(3, "130m", "130Mi"),
			},
			want:    createTortoiseWithVPARecommendation("120m", "120Mi"),
			wantErr: false,
		},
		{
			name: "reduced resources based on VPA recommendation",
			fields: fields{
				preferredReplicaNumUpperLimit: 6,
				suggestedResourceSizeAtMax:    createResourceList("1000m", "1Gi"),
			},
			args: args{
				tortoise: createVerticalTortoiseWithCondition(map[corev1.ResourceName]v1alpha1.ResourceQuantity{
					corev1.ResourceCPU: {
						Quantity: resource.MustParse("120m"),
					},
					corev1.ResourceMemory: {
						Quantity: resource.MustParse("120Mi"),
					},
				}),
				deployment: createDeployment(3, "130m", "130Mi"),
			},
			want:    createVerticalTortoiseWithVPARecommendation("120m", "120Mi"),
			wantErr: false,
		},
		{
			name: "reduced resources based on VPA recommendation when unbalanced container size in multiple containers Pod",
			fields: fields{
				preferredReplicaNumUpperLimit: 6,
				suggestedResourceSizeAtMax:    createResourceList("10000m", "100Gi"),
			},
			args: args{
				tortoise: createTortoiseWithMultipleContainersWithCondition(
					map[corev1.ResourceName]v1alpha1.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("80m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("9Gi"),
						},
					},
					map[corev1.ResourceName]v1alpha1.ResourceQuantity{
						corev1.ResourceCPU: {
							Quantity: resource.MustParse("800m"),
						},
						corev1.ResourceMemory: {
							Quantity: resource.MustParse("0.7Gi"),
						},
					}),
				deployment: createMultipleContainersDeployment(5, "1000m", "1000m", "10Gi", "10Gi"),
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
			want:    createTortoiseWithMultipleContainersWithVPARecommendation("100m", "1000m", "10Gi", "1Gi"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				minimumMinReplicas:            tt.fields.minimumMinReplicas,
				preferredReplicaNumUpperLimit: tt.fields.preferredReplicaNumUpperLimit,
				maxResourceSize:               tt.fields.suggestedResourceSizeAtMax,
			}
			got, err := s.updateVPARecommendation(tt.args.tortoise, tt.args.deployment, tt.args.hpa)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateVPARecommendation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if d := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{}, v1alpha1.Conditions{})); d != "" {
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
func createVerticalTortoise() *v1alpha1.Tortoise {
	return &v1alpha1.Tortoise{
		Spec: v1alpha1.TortoiseSpec{
			ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
				{
					ContainerName: "test-container",
					AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
						corev1.ResourceCPU:    v1alpha1.AutoscalingTypeVertical,
						corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
					},
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				},
			},
		},
	}
}
func createVerticalTortoiseWithCondition(vpaRecommendation map[corev1.ResourceName]v1alpha1.ResourceQuantity) *v1alpha1.Tortoise {
	tortoise := createVerticalTortoise()
	tortoise.Status.Conditions.ContainerRecommendationFromVPA = []v1alpha1.ContainerRecommendationFromVPA{
		{
			ContainerName:  "test-container",
			Recommendation: vpaRecommendation,
		},
	}
	return tortoise
}
func createTortoise() *v1alpha1.Tortoise {
	return &v1alpha1.Tortoise{
		Spec: v1alpha1.TortoiseSpec{
			ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
				{
					ContainerName: "test-container",
					AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
						corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
					},
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				},
			},
		},
	}
}
func createDeployment(replicas int32, cpu, memory string) *v1.Deployment {
	return &v1.Deployment{
		Status: v1.DeploymentStatus{
			Replicas: replicas,
		},
		Spec: v1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Requests: createResourceList(cpu, memory),
							},
						},
					},
				},
			},
		},
	}
}
func createTortoiseWithMultipleContainers() *v1alpha1.Tortoise {
	return &v1alpha1.Tortoise{
		Spec: v1alpha1.TortoiseSpec{
			ResourcePolicy: []v1alpha1.ContainerResourcePolicy{
				{
					ContainerName: "test-container",
					AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
						corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
					},
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				},
				{
					ContainerName: "test-container2",
					AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
						corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
						corev1.ResourceMemory: v1alpha1.AutoscalingTypeHorizontal,
					},
					MinAllocatedResources: createResourceList("100m", "100Mi"),
				},
			},
		},
	}
}
func createMultipleContainersDeployment(replicas int32, cpu1, cpu2, memory1, memory2 string) *v1.Deployment {
	return &v1.Deployment{
		Status: v1.DeploymentStatus{
			Replicas: replicas,
		},
		Spec: v1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Resources: corev1.ResourceRequirements{
								Requests: createResourceList(cpu1, memory1),
							},
						},
						{
							Name: "test-container2",
							Resources: corev1.ResourceRequirements{
								Requests: createResourceList(cpu2, memory2),
							},
						},
					},
				},
			},
		},
	}
}

func createTortoiseWithMultipleContainersWithCondition(vpaRecommendation1, vpaRecommendation2 map[corev1.ResourceName]v1alpha1.ResourceQuantity) *v1alpha1.Tortoise {
	tortoise := createTortoiseWithMultipleContainers()
	tortoise.Status.Conditions.ContainerRecommendationFromVPA = []v1alpha1.ContainerRecommendationFromVPA{
		{
			ContainerName:  "test-container",
			Recommendation: vpaRecommendation1,
		},
		{
			ContainerName:  "test-container2",
			Recommendation: vpaRecommendation2,
		},
	}
	return tortoise
}

func createTortoiseWithCondition(vpaRecommendation map[corev1.ResourceName]v1alpha1.ResourceQuantity) *v1alpha1.Tortoise {
	tortoise := createTortoise()
	tortoise.Status.Conditions.ContainerRecommendationFromVPA = []v1alpha1.ContainerRecommendationFromVPA{
		{
			ContainerName:  "test-container",
			Recommendation: vpaRecommendation,
		},
	}
	return tortoise
}

func createTortoiseWithVPARecommendation(cpu, memory string) *v1alpha1.Tortoise {
	tortoise := createTortoise()
	tortoise.Status.Recommendations = v1alpha1.Recommendations{
		Vertical: &v1alpha1.VerticalRecommendations{
			ContainerResourceRecommendation: []v1alpha1.RecommendedContainerResources{
				{
					ContainerName:       "test-container",
					RecommendedResource: createResourceList(cpu, memory),
				},
			},
		},
	}
	return tortoise
}

func createVerticalTortoiseWithVPARecommendation(cpu, memory string) *v1alpha1.Tortoise {
	tortoise := createVerticalTortoise()
	tortoise.Status.Recommendations = v1alpha1.Recommendations{
		Vertical: &v1alpha1.VerticalRecommendations{
			ContainerResourceRecommendation: []v1alpha1.RecommendedContainerResources{
				{
					ContainerName:       "test-container",
					RecommendedResource: createResourceList(cpu, memory),
				},
			},
		},
	}
	return tortoise
}
func createTortoiseWithMultipleContainersWithVPARecommendation(cpu1, cpu2, memory1, memory2 string) *v1alpha1.Tortoise {
	tortoise := createTortoiseWithMultipleContainers()
	tortoise.Status.Recommendations = v1alpha1.Recommendations{
		Vertical: &v1alpha1.VerticalRecommendations{
			ContainerResourceRecommendation: []v1alpha1.RecommendedContainerResources{
				{
					ContainerName:       "test-container",
					RecommendedResource: createResourceList(cpu1, memory1),
				},
				{
					ContainerName:       "test-container2",
					RecommendedResource: createResourceList(cpu2, memory2),
				},
			},
		},
	}
	return tortoise
}
