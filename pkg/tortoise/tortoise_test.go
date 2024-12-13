package tortoise

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appv1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/mercari/tortoise/api/v1beta3"
)

func TestService_updateUpperRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		tortoise *v1beta3.Tortoise
		vpa      *v1.VerticalPodAutoscaler
		want     *v1beta3.Tortoise
	}{
		{
			name: "success: the current recommendation on tortoise is less than the one on the current VPA",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("1"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("5"),
									},
								},
							},
						},
					},
				},
			},
			vpa: &v1.VerticalPodAutoscaler{
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: &v1.RecommendedPodResources{
						ContainerRecommendations: []v1.RecommendedContainerResources{
							{
								ContainerName: "app",
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1"),
								},
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("8"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("8"),
									corev1.ResourceMemory: resource.MustParse("10"),
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "remove slot for non-existing containers",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("1"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("5"),
									},
								},
							},
							{
								ContainerName: "non-exist",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("1"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("5"),
									},
								},
							},
							{
								ContainerName: "non-exist2",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("1"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("5"),
									},
								},
							},
						},
					},
				},
			},
			vpa: &v1.VerticalPodAutoscaler{
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: &v1.RecommendedPodResources{
						ContainerRecommendations: []v1.RecommendedContainerResources{
							{
								ContainerName: "app",
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1"),
								},
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("8"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("8"),
									corev1.ResourceMemory: resource.MustParse("10"),
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "create a slot for a new container",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "new1",
						},
						{
							ContainerName: "app",
						},
						{
							ContainerName: "new2",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("1"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("5"),
									},
								},
							},
							// no slot for "new"
						},
					},
				},
			},
			vpa: &v1.VerticalPodAutoscaler{
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: &v1.RecommendedPodResources{
						ContainerRecommendations: []v1.RecommendedContainerResources{
							{
								ContainerName: "app",
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1"),
								},
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("8"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("8"),
									corev1.ResourceMemory: resource.MustParse("10"),
								},
							},
							{
								ContainerName: "new1",
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1"),
								},
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("8"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("8"),
									corev1.ResourceMemory: resource.MustParse("10"),
								},
							},
							{
								ContainerName: "new2",
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1"),
								},
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("5"),
									corev1.ResourceMemory: resource.MustParse("8"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("8"),
									corev1.ResourceMemory: resource.MustParse("10"),
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "new1",
						},
						{
							ContainerName: "app",
						},
						{
							ContainerName: "new2",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
							},
							{
								ContainerName: "new1",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
							},
							{
								ContainerName: "new2",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("8"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "success: the current recommendation on tortoise is more than the upper bound on the current VPA",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("1"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("5"),
									},
								},
							},
						},
					},
				},
			},
			vpa: &v1.VerticalPodAutoscaler{
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: &v1.RecommendedPodResources{
						ContainerRecommendations: []v1.RecommendedContainerResources{
							{
								ContainerName: "app",
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("2"),
								},
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("6"),
									corev1.ResourceMemory: resource.MustParse("3"),
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("2"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("2"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU: {
										Quantity: resource.MustParse("5"),
									},
									corev1.ResourceMemory: {
										Quantity: resource.MustParse("2"),
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
			s := &Service{}
			got := s.UpdateContainerRecommendationFromVPA(tt.tortoise, tt.vpa, time.Now())
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreTypes(metav1.Time{})); diff != "" {
				t.Fatalf("diff: %s", diff)
			}
		})
	}
}

func TestService_InitializeTortoise(t *testing.T) {
	timeZone := "Asia/Tokyo"
	jst, err := time.LoadLocation(timeZone)
	if err != nil {
		t.Fatal(err)
	}
	type fields struct {
		rangeOfMinMaxReplicasRecommendationHour int
		minMaxReplicasRoutine                   string
		timeZone                                *time.Location
	}
	tests := []struct {
		name       string
		fields     fields
		tortoise   *v1beta3.Tortoise
		deployment *appv1.Deployment
		want       *v1beta3.Tortoise
	}{
		{
			name: "weekly minMaxReplicasRoutine",
			fields: fields{
				rangeOfMinMaxReplicasRecommendationHour: 8,
				minMaxReplicasRoutine:                   "weekly",
				timeZone:                                jst,
			},
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
				},
			},
			deployment: &appv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: appv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "app",
								},
								{
									Name: "istio",
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					TortoisePhase: v1beta3.TortoisePhaseInitializing,
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
							{
								ContainerName: "istio",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceMemory: v1beta3.ResourcePhase{
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceCPU: v1beta3.ResourcePhase{
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "istio",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceMemory: v1beta3.ResourcePhase{
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceCPU: v1beta3.ResourcePhase{
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Saturday.String()),
									TimeZone: timeZone,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  ptr.To(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  ptr.To(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  ptr.To(time.Saturday.String()),
									TimeZone: timeZone,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "daily minMaxReplicasRoutine",
			fields: fields{
				minMaxReplicasRoutine:                   "daily",
				rangeOfMinMaxReplicasRecommendationHour: 8,
				timeZone:                                jst,
			},
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
				},
			},
			deployment: &appv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: appv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "app",
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					TortoisePhase: v1beta3.TortoisePhaseInitializing,
					Conditions: v1beta3.Conditions{
						ContainerRecommendationFromVPA: []v1beta3.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: v1beta3.ResourcePhase{
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: v1beta3.ResourcePhase{
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
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
			s := &Service{
				rangeOfMinMaxReplicasRecommendationHour: tt.fields.rangeOfMinMaxReplicasRecommendationHour,
				gatheringDataDuration:                   tt.fields.minMaxReplicasRoutine,
				timeZone:                                tt.fields.timeZone,
			}
			got := s.initializeTortoise(tt.tortoise, time.Now())
			if d := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
				t.Errorf("initializeTortoise() diff = %v", d)
			}
		})
	}
}

func TestService_ShouldReconcileTortoiseNow(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name                   string
		lastTimeUpdateTortoise map[client.ObjectKey]time.Time
		tortoise               *v1beta3.Tortoise
		want                   bool
		wantDuration           time.Duration
	}{
		{
			name: "tortoise which isn't recorded lastTimeUpdateTortoise in should be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t2", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
			},
			want: true,
		},
		{
			name: "tortoise which updated a few seconds ago shouldn't be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
			},
			want:         false,
			wantDuration: 59 * time.Second,
		},
		{
			name: "emergency mode un-handled tortoise should be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeEmergency,
				},
			},
			want: true,
		},
		{
			name: "emergency mode tortoise (already handled) is treated as the usual tortoise",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeEmergency,
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseEmergency,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				tortoiseUpdateInterval: 1 * time.Minute,
				lastTimeUpdateTortoise: tt.lastTimeUpdateTortoise,
			}
			got, gotDuration := s.ShouldReconcileTortoiseNow(tt.tortoise, now)
			if got != tt.want {
				t.Errorf("ShouldReconcileTortoiseNow() = %v, want %v", got, tt.want)
			}
			if tt.wantDuration != 0 && gotDuration != tt.wantDuration {
				t.Errorf("ShouldReconcileTortoiseNow() = %v, want %v", gotDuration, tt.wantDuration)
			}
		})
	}
}

func TestService_UpdateTortoiseStatus(t *testing.T) {
	now := time.Now()
	type args struct {
		t   *v1beta3.Tortoise
		now time.Time
	}
	tests := []struct {
		name                       string
		originalTortoise           *v1beta3.Tortoise
		args                       args
		wantLastTimeUpdateTortoise map[client.ObjectKey]time.Time
	}{
		{
			name: "success",
			originalTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
			},
			args: args{
				t: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseInitializing,
					},
				},
				now: now,
			},
			wantLastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				{Name: "t", Namespace: "test"}: now,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := v1beta3.AddToScheme(scheme)
			if err != nil {
				t.Fatalf("failed to add to scheme: %v", err)
			}
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			err = c.Create(context.Background(), tt.originalTortoise)
			if err != nil {
				t.Fatalf("create tortoise: %v", err)
			}
			s := &Service{
				c:                      c,
				lastTimeUpdateTortoise: make(map[client.ObjectKey]time.Time),
			}

			s.updateLastTimeUpdateTortoise(tt.args.t, tt.args.now)
			if d := cmp.Diff(s.lastTimeUpdateTortoise, tt.wantLastTimeUpdateTortoise); d != "" {
				t.Errorf("UpdateTortoiseStatus() diff = %v", d)
			}
		})
	}
}

func TestService_RecordReconciliationFailure(t *testing.T) {
	now := time.Now()
	type args struct {
		t   *v1beta3.Tortoise
		err error
	}
	tests := []struct {
		name string
		args args
		want *v1beta3.Tortoise
	}{
		{
			name: "success reconciliation",
			args: args{
				t: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeFailedToReconcile,
									Status:             corev1.ConditionTrue,
									Message:            "failed to reconcile",
									Reason:             "ReconcileError",
									LastUpdateTime:     metav1.NewTime(now.Add(-1 * time.Minute)),
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								},
							},
						},
					},
				},
				err: nil,
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeFailedToReconcile,
								Status:             corev1.ConditionFalse,
								Message:            "",
								Reason:             "",
								LastUpdateTime:     metav1.NewTime(now),
								LastTransitionTime: metav1.NewTime(now),
							},
						},
					},
				},
			},
		},
		{
			name: "failed reconciliation and tortoise doens't have TortoiseConditionTypeFailedToReconcile",
			args: args{
				t: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								// TortoiseConditionTypeFailedToReconcile isn't recorded yet.
							},
						},
					},
				},
				err: errors.New("failed to reconcile"),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeFailedToReconcile,
								Status:             corev1.ConditionTrue,
								Message:            "failed to reconcile",
								Reason:             "ReconcileError",
								LastUpdateTime:     metav1.NewTime(now),
								LastTransitionTime: metav1.NewTime(now),
							},
						},
					},
				},
			},
		},
		{
			name: "failed reconciliation and tortoise has TortoiseConditionTypeFailedToReconcile",
			args: args{
				t: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeFailedToReconcile,
									Status:             corev1.ConditionFalse,
									Message:            "",
									Reason:             "",
									LastUpdateTime:     metav1.NewTime(now.Add(-1 * time.Minute)),
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								},
							},
						},
					},
				},
				err: errors.New("failed to reconcile"),
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeFailedToReconcile,
								Status:             corev1.ConditionTrue,
								Message:            "failed to reconcile",
								Reason:             "ReconcileError",
								LastUpdateTime:     metav1.NewTime(now),
								LastTransitionTime: metav1.NewTime(now),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				recorder: record.NewFakeRecorder(10),
			}
			if got := s.RecordReconciliationFailure(tt.args.t, tt.args.err, now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Service.RecordReconciliationFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestService_changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(t *testing.T) {
	timeZone := "Asia/Tokyo"
	now := time.Now()
	tests := []struct {
		name                  string
		gatheringDataDuration string
		tortoise              *v1beta3.Tortoise
		want                  *v1beta3.Tortoise
	}{
		{
			name: "minReplicas/maxReplicas recommendation is not yet gathered",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhasePartlyWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now),
								},
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now),
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now),
								},
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									// empty
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									// empty
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhasePartlyWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseWorking, // unchanged because it's vertical.
									LastTransitionTime: metav1.NewTime(now),
								},
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now),
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseWorking, // unchanged
									LastTransitionTime: metav1.NewTime(now),
								},
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									// empty
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									// empty
								},
							},
						},
					},
				},
			},
		},
		{
			name: "some container resource need to gather more data",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseGatheringData,
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 1 * time.Hour)),
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
									// finish gathering
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 8 * time.Hour)),
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								},
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhasePartlyWorking,
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 1 * time.Hour)),
								},
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now),
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								},
								corev1.ResourceMemory: {
									Phase:              v1beta3.ContainerResourcePhaseGatheringData,
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								},
							},
						},
					},
				},
			},
		},
		{
			name:                  "all container resource gathered the data [daily]",
			gatheringDataDuration: "daily",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseGatheringData,
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
									// finish gathering
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 2 * time.Hour)),
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 2 * time.Hour)),
								},
								corev1.ResourceMemory: {
									// off is ignored
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 2 * time.Hour)),
								},
								corev1.ResourceMemory: {
									// off is ignored
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "all container resource gathered the data [weekly]",
			tortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseGatheringData,
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
									// finish gathering
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 8 * time.Hour)),
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 2 * time.Hour)),
								},
								corev1.ResourceMemory: {
									// off is ignored
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     8,
									To:       16,
									TimeZone: timeZone,
									Value:    3,
								},
								{
									From:     16,
									To:       24,
									TimeZone: timeZone,
									Value:    3,
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase:              v1beta3.ContainerResourcePhaseWorking,
									LastTransitionTime: metav1.NewTime(now.Add(-24 * 2 * time.Hour)),
								},
								corev1.ResourceMemory: {
									// off is ignored
									Phase: v1beta3.ContainerResourcePhaseOff,
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
			s := &Service{
				gatheringDataDuration: "weekly",
			}
			if tt.gatheringDataDuration != "" {
				s.gatheringDataDuration = tt.gatheringDataDuration
			}
			got := s.changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(tt.tortoise, now)

			if d := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
				t.Errorf("initializeTortoise() diff = %v", d)
			}
		})
	}
}

func TestUpdateTortoiseAutoscalingPolicyInStatus(t *testing.T) {
	type args struct {
		tortoise *v1beta3.Tortoise
		hpa      *v2.HorizontalPodAutoscaler
	}
	tests := []struct {
		name string
		args args
		want *v1beta3.Tortoise
	}{
		{
			name: "autoscaling policy is not empty",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and all containers have appropriate policy",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								},
							},
							{
								ContainerName: "app2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
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
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
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
		{
			name: "autoscaling policy is empty, and somehow some containers lost resource request",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "app2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "app2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and one policy refers to non-existing container",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								},
							},
							{
								ContainerName: "non-existing",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "app2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "non-existing2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "non-existing2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "app2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "non-existing",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and some containers don't have policy",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								},
							},
							{
								ContainerName: "app2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "new",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "new2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "app2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "new",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "new2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "new",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "new2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "new",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "new2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and some containers don't have policy, but some resources doesn't have resource request",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								},
							},
							{
								ContainerName: "app2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "new",
									Resource: corev1.ResourceList{
										// no cpu
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "new2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("1"),
										// no memory
									},
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseOff,
									},
								},
							},
							{
								ContainerName: "app2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseOff,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff, // Off
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "new",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff, // Off
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "new2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "new",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "new2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "new",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "new2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and some containers don't have policy and we have policy for non-existing container",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								},
							},
							{
								ContainerName: "non-existing",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "app2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "non-existing2",
								Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
									corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
									corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "new",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "new2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseOff,
									},
								},
							},
							{
								ContainerName: "app2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseOff,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "non-existing",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "non-existing2",
								ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
									corev1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									corev1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "new",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "new2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "new",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "new2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "new",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "new2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and hpa is attached",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("hoge"),
						},
					},
					Status: v1beta3.TortoiseStatus{
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
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
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "app",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "app2",
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("hoge"),
					},
				},
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
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "autoscaling policy is empty, and hpa is attached. But some containers don't have resource request",
			args: args{
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("hoge"),
						},
					},
					Status: v1beta3.TortoiseStatus{
						Conditions: v1beta3.Conditions{
							ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
								{
									ContainerName: "app",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
								{
									ContainerName: "app2",
									Resource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
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
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceMemory,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "app",
								},
							},
							{
								Type: v2.ContainerResourceMetricSourceType,
								ContainerResource: &v2.ContainerResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: v2.MetricTarget{
										AverageUtilization: ptr.To[int32](80),
									},
									Container: "app2",
								},
							},
						},
					},
				},
			},
			want: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("hoge"),
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "app2",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							{
								ContainerName: "app2",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
							},
						},
						{
							ContainerName: "app2",
							ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
								corev1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseOff,
								},
								corev1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
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
			got := UpdateTortoiseAutoscalingPolicyInStatus(tt.args.tortoise, tt.args.hpa, time.Now())
			if d := cmp.Diff(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
				t.Errorf("UpdateTortoiseAutoscalingPolicyInStatus() diff = %v", d)
			}
		})
	}
}

func TestService_UpdateResourceRequest(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name         string
		tortoise     *v1beta3.Tortoise
		wantTortoise *v1beta3.Tortoise
		wantErr      bool
	}{
		{
			name: "Tortoise gets ContainerResourceRequests at the first time (tortoise is Auto)",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					// No ContainerResourceRequests is set.
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
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
		{
			name: "ContainerResourceRequests won't be set if the autoscaling policy is Off",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("0"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("0Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeOff,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"), // off is ignored.
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"), // off is ignored.
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("0"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("0Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "ContainerResourceRequests won't be set if the recommendation is zero",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("0"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("0Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"), // 0 is ignored.
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"), // 0 is ignored.
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("0"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("0Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "The recommendation is smaller than before, and we recently update the value. podShouldBeUpdatedWithNewResource:false is returned",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:   v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status: corev1.ConditionTrue,
								// Very recently
								LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
								LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
								LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
								Message:            "The recommendation is provided",
							},
						},
						// Not updated because we recently updated the value.
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
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
		{
			name: "The recommendation is smaller than before, but we don't recently update the value. podShouldBeUpdatedWithNewResource:true is returned",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:   v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status: corev1.ConditionTrue,
								// Not recently
								LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
								LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							// updated
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
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
		{
			name: "ContainerResourceRequests is not modified and podShouldBeUpdatedWithNewResource:false is always returned",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							// updated
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("1Gi"),
										corev1.ResourceCPU:    resource.MustParse("1"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
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
		{
			name: "ContainerResourceRequests is NOT modified when tortoise is Off mode",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeOff,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						// empty
						ContainerResourceRequests: nil,
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeOff,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is not provided because it's Off mode",
							},
						},
						// still empty because it's Off.
						ContainerResourceRequests: nil,
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "ContainerResourceRequests is removed when tortoise is Off mode (= probably this tortoise is changed back from auto to off)",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeOff,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeOff,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionFalse, // changed to false
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is not provided because it's Off mode",
							},
						},
						// unchange
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse("1"),
								},
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Tortoise's mode changed: Auto → Off → Auto",
			tortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is not provided because it's Off mode",
							},
						},
						ContainerResourceRequests: nil,
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "sidecar",
							Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
								corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Recommendations: v1beta3.Recommendations{
						Vertical: v1beta3.VerticalRecommendations{
							ContainerResourceRecommendation: []v1beta3.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
								{
									ContainerName: "sidecar",
									RecommendedResource: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"),
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metav1.NewTime(now),
								LastUpdateTime:     metav1.NewTime(now),
								Message:            "The recommendation is provided",
							},
						},
						ContainerResourceRequests: []v1beta3.ContainerResourceRequests{
							{
								ContainerName: "app",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
							{
								ContainerName: "sidecar",
								Resource: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
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
			c := &Service{
				recorder: record.NewFakeRecorder(10),
			}

			gotTortoise, err := c.UpdateResourceRequest(context.Background(), tt.tortoise.DeepCopy(), 10, now)
			if (err != nil) != tt.wantErr {
				t.Errorf("Service.UpdateResourceRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(gotTortoise, tt.wantTortoise); diff != "" {
				t.Errorf("Service.UpdateResourceRequest() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestService_UpdateTortoisePhaseIfHPAIsUnhealthy(t *testing.T) {
	type args struct {
		t             *v1beta3.Tortoise
		scalingActive bool
	}
	tests := []struct {
		name         string
		args         args
		wantTortoise *v1beta3.Tortoise
	}{
		{
			name: "healthy HPA tortoise working",
			args: args{
				t: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
					},
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
				},
				scalingActive: true,
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test", ResourceVersion: "1"},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
			},
		},
		{
			name: "unhealthy HPA tortoise working",
			args: args{
				t: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
					},
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
				},
				scalingActive: false,
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test", ResourceVersion: "1"},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseEmergency,
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
			},
		},
		{
			name: "unhealthy HPA tortoise off",
			args: args{
				t: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
					},
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeOff,
					},
				},
				scalingActive: false,
			},
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test", ResourceVersion: "1"},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
				},
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeOff,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := v1beta3.AddToScheme(scheme)
			if err != nil {
				t.Fatalf("failed to add to scheme: %v", err)
			}
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			err = c.Create(context.Background(), tt.args.t)
			if err != nil {
				t.Fatalf("create tortoise: %v", err)
			}
			s := &Service{
				c:                      c,
				lastTimeUpdateTortoise: make(map[client.ObjectKey]time.Time),
			}

			tt.args.t, err := s.UpdateTortoisePhaseIfHPAIsUnhealthy(context.Background(), tt.args.scalingActive, tt.args.t)
			if err != nil {
				t.Fatalf("failed to update tortoise phase: %v", err)
			}
			if d := cmp.Diff(tt.args.t, tt.wantTortoise); d != "" {
				t.Errorf("UpdateTortoiseStatus() diff = %v", d)
			}
		})
	}
}
