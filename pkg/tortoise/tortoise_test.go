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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/mercari/tortoise/api/v1beta1"
)

func TestService_updateUpperRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		tortoise *v1beta1.Tortoise
		vpa      *v1.VerticalPodAutoscaler
		want     *v1beta1.Tortoise
	}{
		{
			name: "success: the current recommendation on tortoise is less than the one on the current VPA",
			tortoise: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						ContainerRecommendationFromVPA: []v1beta1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("1"),
									},
									"mem": {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("5"),
									},
									"mem": {
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
									"cpu": resource.MustParse("1"),
									"mem": resource.MustParse("1"),
								},
								Target: map[corev1.ResourceName]resource.Quantity{
									"cpu": resource.MustParse("5"),
									"mem": resource.MustParse("8"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									"cpu": resource.MustParse("8"),
									"mem": resource.MustParse("10"),
								},
							},
						},
					},
				},
			},
			want: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						ContainerRecommendationFromVPA: []v1beta1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("5"),
									},
									"mem": {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("5"),
									},
									"mem": {
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
			tortoise: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						ContainerRecommendationFromVPA: []v1beta1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("1"),
									},
									"mem": {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("5"),
									},
									"mem": {
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
									"cpu": resource.MustParse("2"),
									"mem": resource.MustParse("2"),
								},
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									"cpu": resource.MustParse("1"),
									"mem": resource.MustParse("1"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									"cpu": resource.MustParse("6"),
									"mem": resource.MustParse("3"),
								},
							},
						},
					},
				},
			},
			want: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						ContainerRecommendationFromVPA: []v1beta1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("2"),
									},
									"mem": {
										Quantity: resource.MustParse("2"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("5"),
									},
									"mem": {
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
			got := s.UpdateUpperRecommendation(tt.tortoise, tt.vpa)
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreTypes(metav1.Time{})) {
				t.Errorf("updateUpperRecommendation() = %v, want %v", got, tt.want)
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
		tortoise   *v1beta1.Tortoise
		deployment *appv1.Deployment
		want       *v1beta1.Tortoise
	}{
		{
			name: "weekly minMaxReplicasRoutine",
			fields: fields{
				rangeOfMinMaxReplicasRecommendationHour: 8,
				minMaxReplicasRoutine:                   "weekly",
				timeZone:                                jst,
			},
			tortoise: &v1beta1.Tortoise{
				Spec: v1beta1.TortoiseSpec{
					ResourcePolicy: []v1beta1.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1beta1.AutoscalingType{
								corev1.ResourceMemory: v1beta1.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta1.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio",
							AutoscalingPolicy: map[corev1.ResourceName]v1beta1.AutoscalingType{
								corev1.ResourceMemory: v1beta1.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta1.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1beta1.TortoiseStatus{},
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
			want: &v1beta1.Tortoise{
				Spec: v1beta1.TortoiseSpec{
					ResourcePolicy: []v1beta1.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1beta1.AutoscalingType{
								corev1.ResourceMemory: v1beta1.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta1.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio",
							AutoscalingPolicy: map[corev1.ResourceName]v1beta1.AutoscalingType{
								corev1.ResourceMemory: v1beta1.AutoscalingTypeOff,
								corev1.ResourceCPU:    v1beta1.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1beta1.TortoiseStatus{
					TortoisePhase: v1beta1.TortoisePhaseInitializing,
					Targets:       v1beta1.TargetsStatus{Deployment: "deployment"},
					Conditions: v1beta1.Conditions{
						ContainerRecommendationFromVPA: []v1beta1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
							{
								ContainerName: "istio",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta1.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta1.ContainerResourcePhase{
								corev1.ResourceMemory: v1beta1.ContainerResourcePhaseGatheringData,
								corev1.ResourceCPU:    v1beta1.ContainerResourcePhaseGatheringData,
							},
						},
						{
							ContainerName: "istio",
							ResourcePhases: map[corev1.ResourceName]v1beta1.ContainerResourcePhase{
								corev1.ResourceMemory: v1beta1.ContainerResourcePhaseOff,
								corev1.ResourceCPU:    v1beta1.ContainerResourcePhaseGatheringData,
							},
						},
					},
					Recommendations: v1beta1.Recommendations{
						Horizontal: v1beta1.HorizontalRecommendations{
							MinReplicas: []v1beta1.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Saturday.String()),
									TimeZone: timeZone,
								},
							},
							MaxReplicas: []v1beta1.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Sunday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Monday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Tuesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Wednesday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Thursday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Friday.String()),
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  pointer.String(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  pointer.String(time.Saturday.String()),
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  pointer.String(time.Saturday.String()),
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
			tortoise: &v1beta1.Tortoise{
				Spec: v1beta1.TortoiseSpec{
					ResourcePolicy: []v1beta1.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1beta1.AutoscalingType{
								corev1.ResourceMemory: v1beta1.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta1.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1beta1.TortoiseStatus{},
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
			want: &v1beta1.Tortoise{
				Spec: v1beta1.TortoiseSpec{
					ResourcePolicy: []v1beta1.ContainerResourcePolicy{
						{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1beta1.AutoscalingType{
								corev1.ResourceMemory: v1beta1.AutoscalingTypeVertical,
								corev1.ResourceCPU:    v1beta1.AutoscalingTypeHorizontal,
							},
						},
					},
				},
				Status: v1beta1.TortoiseStatus{
					TortoisePhase: v1beta1.TortoisePhaseInitializing,
					Targets:       v1beta1.TargetsStatus{Deployment: "deployment"},
					Conditions: v1beta1.Conditions{
						ContainerRecommendationFromVPA: []v1beta1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
						},
					},
					ContainerResourcePhases: []v1beta1.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[corev1.ResourceName]v1beta1.ContainerResourcePhase{
								corev1.ResourceCPU:    v1beta1.ContainerResourcePhaseGatheringData,
								corev1.ResourceMemory: v1beta1.ContainerResourcePhaseGatheringData,
							},
						},
					},
					Recommendations: v1beta1.Recommendations{
						Horizontal: v1beta1.HorizontalRecommendations{
							MinReplicas: []v1beta1.ReplicasRecommendation{
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
							MaxReplicas: []v1beta1.ReplicasRecommendation{
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
				minMaxReplicasRoutine:                   tt.fields.minMaxReplicasRoutine,
				timeZone:                                tt.fields.timeZone,
			}
			got := s.initializeTortoise(tt.tortoise, tt.deployment)
			if d := cmp.Diff(got, tt.want); d != "" {
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
		tortoise               *v1beta1.Tortoise
		want                   bool
		wantDuration           time.Duration
	}{
		{
			name: "tortoise which isn't recorded lastTimeUpdateTortoise in should be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t2", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta1.TortoiseSpec{
					UpdateMode: v1beta1.AutoUpdateMode,
				},
			},
			want: true,
		},
		{
			name: "tortoise which updated a few seconds ago shouldn't be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta1.TortoiseSpec{
					UpdateMode: v1beta1.AutoUpdateMode,
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
			tortoise: &v1beta1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta1.TortoiseSpec{
					UpdateMode: v1beta1.UpdateModeEmergency,
				},
			},
			want: true,
		},
		{
			name: "emergency mode tortoise (already handled) is treated as the usual tortoise",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1beta1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1beta1.TortoiseSpec{
					UpdateMode: v1beta1.UpdateModeEmergency,
				},
				Status: v1beta1.TortoiseStatus{
					TortoisePhase: v1beta1.TortoisePhaseEmergency,
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
		t   *v1beta1.Tortoise
		now time.Time
	}
	tests := []struct {
		name                       string
		originalTortoise           *v1beta1.Tortoise
		args                       args
		wantLastTimeUpdateTortoise map[client.ObjectKey]time.Time
	}{
		{
			name: "success",
			originalTortoise: &v1beta1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
			},
			args: args{
				t: &v1beta1.Tortoise{
					ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "test"},
					Status: v1beta1.TortoiseStatus{
						TortoisePhase: v1beta1.TortoisePhaseInitializing,
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
			err := v1beta1.AddToScheme(scheme)
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
		t   *v1beta1.Tortoise
		err error
	}
	tests := []struct {
		name string
		args args
		want *v1beta1.Tortoise
	}{
		{
			name: "success reconciliation",
			args: args{
				t: &v1beta1.Tortoise{
					Status: v1beta1.TortoiseStatus{
						Conditions: v1beta1.Conditions{
							TortoiseConditions: []v1beta1.TortoiseCondition{
								{
									Type:               v1beta1.TortoiseConditionTypeFailedToReconcile,
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
			want: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						TortoiseConditions: []v1beta1.TortoiseCondition{
							{
								Type:               v1beta1.TortoiseConditionTypeFailedToReconcile,
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
				t: &v1beta1.Tortoise{
					Status: v1beta1.TortoiseStatus{
						Conditions: v1beta1.Conditions{
							TortoiseConditions: []v1beta1.TortoiseCondition{
								// TortoiseConditionTypeFailedToReconcile isn't recorded yet.
							},
						},
					},
				},
				err: errors.New("failed to reconcile"),
			},
			want: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						TortoiseConditions: []v1beta1.TortoiseCondition{
							{
								Type:               v1beta1.TortoiseConditionTypeFailedToReconcile,
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
				t: &v1beta1.Tortoise{
					Status: v1beta1.TortoiseStatus{
						Conditions: v1beta1.Conditions{
							TortoiseConditions: []v1beta1.TortoiseCondition{
								{
									Type:               v1beta1.TortoiseConditionTypeFailedToReconcile,
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
			want: &v1beta1.Tortoise{
				Status: v1beta1.TortoiseStatus{
					Conditions: v1beta1.Conditions{
						TortoiseConditions: []v1beta1.TortoiseCondition{
							{
								Type:               v1beta1.TortoiseConditionTypeFailedToReconcile,
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
