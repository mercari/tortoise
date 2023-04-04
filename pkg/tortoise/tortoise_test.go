package tortoise

import (
	appv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mercari/tortoise/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

func TestService_updateUpperRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		tortoise *v1alpha1.Tortoise
		vpa      *v1.VerticalPodAutoscaler
		want     *v1alpha1.Tortoise
	}{
		{
			name: "success: the current recommendation on tortoise is less than the one on the current VPA",
			tortoise: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("1"),
									},
									"mem": {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("5"),
									},
									"mem": {
										Quantity: resource.MustParse("8"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
			tortoise: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("1"),
									},
									"mem": {
										Quantity: resource.MustParse("1"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
									"cpu": {
										Quantity: resource.MustParse("2"),
									},
									"mem": {
										Quantity: resource.MustParse("2"),
									},
								},
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
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
		timeZone                                *time.Location
	}
	tests := []struct {
		name       string
		fields     fields
		tortoise   *v1alpha1.Tortoise
		deployment *appv1.Deployment
		want       *v1alpha1.Tortoise
	}{
		{
			fields: fields{
				rangeOfMinMaxReplicasRecommendationHour: 8,
				timeZone:                                jst,
			},
			tortoise: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{},
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
			want: &v1alpha1.Tortoise{
				Status: v1alpha1.TortoiseStatus{
					TortoisePhase: v1alpha1.TortoisePhaseInitializing,
					Targets:       v1alpha1.TargetsStatus{Deployment: "deployment"},
					Conditions: v1alpha1.Conditions{
						ContainerRecommendationFromVPA: []v1alpha1.ContainerRecommendationFromVPA{
							{
								ContainerName: "app",
								Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
								MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
									corev1.ResourceCPU:    {},
									corev1.ResourceMemory: {},
								},
							},
						},
					},
					Recommendations: v1alpha1.Recommendations{
						Horizontal: &v1alpha1.HorizontalRecommendations{
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									WeekDay:  time.Sunday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Sunday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Sunday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Monday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Monday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Monday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Tuesday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Tuesday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Tuesday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Wednesday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Wednesday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Wednesday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Thursday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Thursday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Thursday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Friday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Friday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Friday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Saturday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Saturday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Saturday,
									TimeZone: timeZone,
								},
							},
							MaxReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:     0,
									To:       8,
									WeekDay:  time.Sunday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Sunday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Sunday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Monday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Monday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Monday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Tuesday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Tuesday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Tuesday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Wednesday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Wednesday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Wednesday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Thursday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Thursday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Thursday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Friday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Friday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Friday,
									TimeZone: timeZone,
								},
								{
									From:     0,
									To:       8,
									WeekDay:  time.Saturday,
									TimeZone: timeZone,
								},
								{
									From:     8,
									To:       16,
									WeekDay:  time.Saturday,
									TimeZone: timeZone,
								},
								{
									From:     16,
									To:       24,
									WeekDay:  time.Saturday,
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
		tortoise               *v1alpha1.Tortoise
		want                   bool
		wantDuration           time.Duration
	}{
		{
			name: "tortoise which isn't recorded lastTimeUpdateTortoise in should be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t2", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1alpha1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1alpha1.TortoiseSpec{
					UpdateMode: v1alpha1.AutoUpdateMode,
				},
			},
			want: true,
		},
		{
			name: "tortoise which updated a few seconds ago shouldn't be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1alpha1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1alpha1.TortoiseSpec{
					UpdateMode: v1alpha1.AutoUpdateMode,
				},
			},
			want:         false,
			wantDuration: 59 * time.Second,
		},
		{
			name: "emergency mode tortoise should be updated",
			lastTimeUpdateTortoise: map[client.ObjectKey]time.Time{
				client.ObjectKey{Name: "t", Namespace: "default"}: now.Add(-1 * time.Second),
			},
			tortoise: &v1alpha1.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t",
					Namespace: "default",
				},
				Spec: v1alpha1.TortoiseSpec{
					UpdateMode: v1alpha1.UpdateModeEmergency,
				},
			},
			want: true,
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
