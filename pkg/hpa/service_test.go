package hpa

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/mercari/tortoise/api/v1beta3"
)

const (
	defaultEmergencyModeGracePeriod = 5 * time.Minute
)

func TestClient_UpdateHPAFromTortoiseRecommendation(t *testing.T) {
	now := metav1.NewTime(time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC))

	type args struct {
		ctx      context.Context
		tortoise *v1beta3.Tortoise
		now      time.Time
	}
	tests := []struct {
		name               string
		args               args
		excludeMetricRegex string
		initialHPA         *v2.HorizontalPodAutoscaler
		want               *v2.HorizontalPodAutoscaler
		wantTortoise       *v1beta3.Tortoise
		wantErr            bool
	}{
		{
			name: "Basic test case with container resource metrics",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
							Type: v2.ExternalMetricSourceType,
							// should be kept
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "kept",
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
					MinReplicas: ptrInt32(3),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ExternalMetricSourceType,
							// should be kept
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "kept",
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "exclude external metrics correctly",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
							},
						},
					},
				},
				now: now.Time,
			},
			excludeMetricRegex: ".*-exclude-metric",
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ExternalMetricSourceType,
							// should be kept
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "kept",
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							// should be excluded
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "hogehoge-exclude-metric",
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
					MinReplicas: ptrInt32(3),
					MaxReplicas: 6,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ExternalMetricSourceType,
							// should be kept
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "kept",
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceMemory,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "maximum minReplica is applied",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     10000,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     10000,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
					MinReplicas: ptrInt32(1000), // maximum minReplica
					MaxReplicas: 10000,
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     10000,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     10000,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "global maximum maxReplica is applied",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     999999,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
					MinReplicas: ptrInt32(3),
					MaxReplicas: 10001, // maximum maxReplica
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     999999,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "tortoise maximum maxReplica is applied",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode:  v1beta3.UpdateModeAuto,
						MaxReplicas: ptrInt32(9999),
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     999999, // too big
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
					MinReplicas: ptrInt32(3),
					MaxReplicas: 9999, // maximum maxReplica
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					UpdateMode:  v1beta3.UpdateModeAuto,
					MaxReplicas: ptrInt32(9999),
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     999999,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "the change is limited by tortoiseHPATargetUtilizationMaxIncrease",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](10),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](60), // 80 is recommended but only +50 is allowed.
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
			name: "no update preformed when we recently updated it",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Conditions: v1beta3.Conditions{
							TortoiseConditions: []v1beta3.TortoiseCondition{
								{
									Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-1 * time.Minute)),
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Spec: v1beta3.TortoiseSpec{
					UpdateMode: v1beta3.UpdateModeAuto,
				},
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
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
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeOff,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
			name: "no update preformed when all horizontal is unready",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeOff,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseGatheringData,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
			name: "no update preformed when ContainerResourcePhases isn't working",
			args: args{
				ctx: context.Background(),
				tortoise: &v1beta3.Tortoise{
					Spec: v1beta3.TortoiseSpec{
						UpdateMode: v1beta3.UpdateModeAuto,
					},
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhasePartlyWorking,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
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
									AverageUtilization: ptr.To[int32](90), // updated
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50), // not updated
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
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseEmergency,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
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
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseBackToNormal,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
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
				tortoise: &v1beta3.Tortoise{
					Status: v1beta3.TortoiseStatus{
						TortoisePhase: v1beta3.TortoisePhaseBackToNormal,
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: v1beta3.Recommendations{
							Horizontal: v1beta3.HorizontalRecommendations{
								TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []v1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     3,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
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
									AverageUtilization: ptr.To[int32](60),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](50),
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
					Behavior:    defaultHPABehaviorValue.DeepCopy(),
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
									AverageUtilization: ptr.To[int32](90),
								},
								Container: "app",
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](80),
								},
								Container: "istio-proxy",
							},
						},
					},
				},
			},
			wantTortoise: &v1beta3.Tortoise{
				Status: v1beta3.TortoiseStatus{
					TortoisePhase: v1beta3.TortoisePhaseWorking,
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeHorizontal,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
							},
						},
					},
					Conditions: v1beta3.Conditions{
						TortoiseConditions: []v1beta3.TortoiseCondition{
							{
								Type:               v1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: v1beta3.Recommendations{
						Horizontal: v1beta3.HorizontalRecommendations{
							TargetUtilizations: []v1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []v1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     3,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
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
			c, err := New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90, 50, time.Hour, nil, 1000, 10001, 3, tt.excludeMetricRegex, 5*time.Minute, false, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
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

func TestService_IsGlobalDisableModeEnabled(t *testing.T) {
	tests := []struct {
		name              string
		globalDisableMode bool
		expectedResult    bool
	}{
		{
			name:              "global disable mode enabled",
			globalDisableMode: true,
			expectedResult:    true,
		},
		{
			name:              "global disable mode disabled",
			globalDisableMode: false,
			expectedResult:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &Service{
				globalDisableMode: tt.globalDisableMode,
			}
			result := service.IsGlobalDisableModeEnabled()
			if result != tt.expectedResult {
				t.Errorf("IsGlobalDisableModeEnabled() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestService_InitializeHPA(t *testing.T) {
	type args struct {
		tortoise   *v1beta3.Tortoise
		replicaNum int32
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
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind:       "Deployment",
								Name:       "deployment",
								APIVersion: "apps/v1",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
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
					},
				},
				replicaNum: 4,
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise-hpa-tortoise",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(3),
					MaxReplicas: 1000,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
					},
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
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
								},
							},
						},
					},
				},
				replicaNum: 4,
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
		{
			name: "set tortoise hpa to existing even if all autoscaling policy is vertical",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind:       "Deployment",
								Name:       "deployment",
								APIVersion: "apps/v1",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								},
							},
						},
					},
				},
				replicaNum: 4,
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
			c, err := New(fake.NewClientBuilder().Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, nil, 100, 1000, 3, "", 5*time.Minute, false, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if tt.initialHPA != nil {
				c, err = New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, nil, 100, 1000, 3, "", 5*time.Minute, false, nil)
				if err != nil {
					t.Fatalf("New() error = %v", err)
				}
			}
			_, err = c.InitializeHPA(context.Background(), tt.args.tortoise, tt.args.replicaNum, time.Now())
			if (err != nil) != tt.wantErr {
				t.Errorf("Service.InitializeHPA() error = %v, wantErr %v", err, tt.wantErr)
			}
			hpa := &v2.HorizontalPodAutoscaler{}
			err = c.c.Get(context.Background(), client.ObjectKey{Name: tt.afterHPA.Name, Namespace: tt.afterHPA.Namespace}, hpa)
			if err != nil {
				t.Errorf("get hpa error = %v", err)
			}

			if d := cmp.Diff(tt.afterHPA, hpa, cmpopts.IgnoreFields(v2.HorizontalPodAutoscaler{}, "TypeMeta"), cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"), cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
				t.Errorf("Service.InitializeHPA() hpa diff = %v", d)
			}
		})
	}
}

func TestService_UpdateHPASpecFromTortoiseAutoscalingPolicy(t *testing.T) {
	type args struct {
		tortoise   *v1beta3.Tortoise
		replicaNum int32
	}
	tests := []struct {
		name string
		// initialHPA is the initial state of the HPA in kube-apiserver
		initialHPA   *v2.HorizontalPodAutoscaler
		args         args
		afterHPA     *v2.HorizontalPodAutoscaler
		wantTortoise *v1beta3.Tortoise
		wantErr      bool
	}{
		{
			name: "add metrics to existing hpa",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "existing-hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "existing-hpa",
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
		{
			name: "remove metrics from hpa",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
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
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
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
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
		{
			name: "when remove all horizontal, hpa would be removed if it's created by Tortoise",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						DeletionPolicy: v1beta3.DeletionPolicyDeleteAll,
						TargetRefs: v1beta3.TargetRefs{
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					DeletionPolicy: v1beta3.DeletionPolicyDeleteAll,
					TargetRefs: v1beta3.TargetRefs{
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: nil, // hpa would be removed
		},
		{
			name: "when remove all horizontal, hpa would be disabled if it's created by users",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						DeletionPolicy: v1beta3.DeletionPolicyDeleteAll,
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
							{
								ContainerName: "istio-proxy",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
								},
							},
						},
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					DeletionPolicy: v1beta3.DeletionPolicyDeleteAll,
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
						{
							ContainerName: "istio-proxy",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
							},
						},
					},
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(4), // disabled
					MaxReplicas: 4,           // disabled
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
		{
			name: "remove metrics from hpa, but not remove the external metrics",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
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
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "external-metric",
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
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
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "external-metric",
								},
							},
						},
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
		{
			name: "add metrics to existing hpa (with external metrics)",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "external-metric",
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "external-metric",
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
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
		{
			name: "Resource type metric is removed",
			args: args{
				tortoise: &v1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: v1beta3.TortoiseSpec{
						TargetRefs: v1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: v1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: v1beta3.TortoiseStatus{
						AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
							{
								ContainerName: "app",
								Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
									v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
									v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
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
						ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: v1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: v1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
					},
				},
				replicaNum: 4,
			},
			initialHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ResourceMetricSourceType,
							Resource: &v2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "external-metric",
								},
							},
						},
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
			wantTortoise: &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: v1beta3.TortoiseStatus{
					AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
						{
							ContainerName: "app",
							Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
								v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
								v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
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
					ContainerResourcePhases: []v1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]v1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: v1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: v1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa",
					Namespace: "default",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(1),
					MaxReplicas: 2,
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "deployment",
						APIVersion: "apps/v1",
					},
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ExternalMetricSourceType,
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "external-metric",
								},
							},
						},
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
			c, err := New(fake.NewClientBuilder().Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, nil, 1000, 10000, 3, "", 5*time.Minute, false, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if tt.initialHPA != nil {
				c, err = New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, nil, 1000, 10000, 3, "", 5*time.Minute, false, nil)
				if err != nil {
					t.Fatalf("New() error = %v", err)
				}
			}
			var givenHPA *v2.HorizontalPodAutoscaler
			if tt.args.tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
				// givenHPA is only non-nil when the tortoise has a reference to an existing HPA
				givenHPA = tt.initialHPA
			}
			tortoise, err := c.UpdateHPASpecFromTortoiseAutoscalingPolicy(context.Background(), tt.args.tortoise, givenHPA, tt.args.replicaNum, time.Now())
			if (err != nil) != tt.wantErr {
				t.Errorf("Service.UpdateHPASpecFromTortoiseAutoscalingPolicy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if d := cmp.Diff(tt.wantTortoise, tortoise, cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
				t.Errorf("Service.UpdateHPASpecFromTortoiseAutoscalingPolicy() tortoise diff = %v", d)
			}

			if tt.afterHPA == nil {
				// hpa should be removed
				hpa := &v2.HorizontalPodAutoscaler{}
				err = c.c.Get(context.Background(), client.ObjectKey{Name: tt.initialHPA.Name, Namespace: tt.initialHPA.Namespace}, hpa)
				if err == nil || !apierrors.IsNotFound(err) {
					t.Errorf("hpa should be removed")
				}
				return
			}

			hpa := &v2.HorizontalPodAutoscaler{}
			err = c.c.Get(context.Background(), client.ObjectKey{Name: tt.afterHPA.Name, Namespace: tt.afterHPA.Namespace}, hpa)
			if err != nil {
				t.Errorf("get hpa error = %v", err)
			}
			if d := cmp.Diff(tt.afterHPA, hpa, cmpopts.IgnoreFields(v2.HorizontalPodAutoscaler{}, "TypeMeta"), cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")); d != "" {
				t.Errorf("Service.UpdateHPASpecFromTortoiseAutoscalingPolicy() hpa diff = %v", d)
			}
		})
	}
}

func TestService_IsHpaMetricAvailable(t *testing.T) {
	commonTortoise := &v1beta3.Tortoise{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tortoise",
			Namespace: "default",
		},
		Status: v1beta3.TortoiseStatus{
			AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
				{
					ContainerName: "app",
					Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
						v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
					},
				},
			},
		},
	}

	verticalOnlyTortoise := &v1beta3.Tortoise{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tortoise-vertical",
			Namespace: "default",
		},
		Status: v1beta3.TortoiseStatus{
			AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
				{
					ContainerName: "app",
					Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
						v1.ResourceCPU:    v1beta3.AutoscalingTypeVertical,
						v1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		Tortoise *v1beta3.Tortoise
		HPA      *v2.HorizontalPodAutoscaler
		result   bool
	}{
		{
			name:     "metric server down, should return false",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Status:  "True",
							Type:    v2.AbleToScale,
							Message: "recommended size matches current size",
						},
						{
							Status:  "False",
							Type:    v2.ScalingActive,
							Message: "the HPA was unable to compute the replica count: failed to get cpu utilization",
						},
						{
							Status:  "False",
							Type:    v2.ScalingLimited,
							Message: "the desired count is within the acceptable range",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
								Name: v1.ResourceCPU,
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "istio-proxy",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
								Name: v1.ResourceCPU,
							},
						},
					},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(3),
					MaxReplicas: 1000,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
					},
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
			result: false,
		},
		{
			name:     "Container resource metric missing",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Status:  "True",
							Type:    v2.AbleToScale,
							Message: "recommended size matches current size",
						},
						{
							Status:  "True",
							Type:    v2.ScalingActive,
							Message: "the HPA was able to successfully calculate a replica count from cpu container resource utilization (percentage of request)",
						},
						{
							Status:  "False",
							Type:    v2.ScalingLimited,
							Message: "the desired count is within the acceptable range",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
								Name: v1.ResourceCPU,
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "istio-proxy",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
								Name: v1.ResourceCPU,
							},
						},
					},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(3),
					MaxReplicas: 1000,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
					},
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
			result: false,
		},
		{
			name:     "HPA working normally",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Status:  "True",
							Type:    v2.AbleToScale,
							Message: "recommended size matches current size",
						},
						{
							Status:  "True",
							Type:    v2.ScalingActive,
							Message: "the HPA was able to successfully calculate a replica count from cpu container resource utilization (percentage of request)",
						},
						{
							Status:  "False",
							Type:    v2.ScalingLimited,
							Message: "the desired count is within the acceptable range",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](70),
									AverageValue:       resource.NewQuantity(5, resource.DecimalSI),
									Value:              resource.NewQuantity(5, resource.DecimalSI),
								},
								Name: v1.ResourceCPU,
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "istio-proxy",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](70),
									AverageValue:       resource.NewQuantity(5, resource.DecimalSI),
									Value:              resource.NewQuantity(5, resource.DecimalSI),
								},
								Name: v1.ResourceCPU,
							},
						},
					},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					MinReplicas: ptrInt32(3),
					MaxReplicas: 1000,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "app",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name:      v1.ResourceCPU,
								Container: "istio-proxy",
								Target: v2.MetricTarget{
									AverageUtilization: ptr.To[int32](70),
									Type:               v2.UtilizationMetricType,
								},
							},
						},
					},
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
			result: true,
		},
		{
			name:     "all policies are vertical, should return true",
			Tortoise: verticalOnlyTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:   "ScalingActive",
							Status: "False",
							Reason: "FailedGetResourceMetric",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: v2.ContainerResourceMetricSourceType,
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Name:      v1.ResourceCPU,
								Container: "app",
								Current: v2.MetricValueStatus{
									Value: resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			result: true,
		},
		{
			name:     "all policies are vertical with nil HPA, should return true",
			Tortoise: verticalOnlyTortoise,
			HPA:      nil,
			result:   true,
		},
		{
			name:     "all policies are vertical with empty HPA status, should return true",
			Tortoise: verticalOnlyTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{},
			},
			result: true,
		},
		{
			name:     "HPA with non-zero external metric and non-zero container metric, should return true",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:   "ScalingActive",
							Status: "True",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "External",
							External: &v2.ExternalMetricStatus{
								Current: v2.MetricValueStatus{
									Value: resource.NewQuantity(500, resource.DecimalSI),
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](50),
									AverageValue:       resource.NewQuantity(1, resource.DecimalSI),
									Value:              resource.NewQuantity(1, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			result: true,
		},
		{
			name:     "HPA with zero external metric and non-zero container metric, should return true",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:   "ScalingActive",
							Status: "True",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "External",
							External: &v2.ExternalMetricStatus{
								Current: v2.MetricValueStatus{
									Value: resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](50),
									AverageValue:       resource.NewQuantity(1, resource.DecimalSI),
									Value:              resource.NewQuantity(1, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			result: true,
		},
		{
			name:     "HPA with non-zero external metric and zero container metric, should return true",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:   "ScalingActive",
							Status: "True",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "External",
							External: &v2.ExternalMetricStatus{
								Current: v2.MetricValueStatus{
									Value: resource.NewQuantity(500, resource.DecimalSI),
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			result: true,
		},
		{
			name:     "HPA with zero external metric and zero container metric, should return false",
			Tortoise: commonTortoise,
			HPA: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:   "ScalingActive",
							Status: "True",
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "External",
							External: &v2.ExternalMetricStatus{
								Current: v2.MetricValueStatus{
									Value: resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			result: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(fake.NewClientBuilder().Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, nil, 100, 1000, 3, "", 5*time.Minute, false, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			status := c.IsHpaMetricAvailable(context.Background(), tt.Tortoise, tt.HPA)
			if status != tt.result {
				t.Errorf("Service.checkHpaMetricStatus() status test: %s failed", tt.name)
				return
			}
		})
	}
}

func TestService_IsHpaMetricAvailable_EmergencyModeGracePeriod(t *testing.T) {
	now := time.Now()

	// Tortoise with horizontal scaling policy
	horizontalTortoise := &v1beta3.Tortoise{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tortoise",
			Namespace: "default",
		},
		Status: v1beta3.TortoiseStatus{
			AutoscalingPolicy: []v1beta3.ContainerAutoscalingPolicy{
				{
					ContainerName: "app",
					Policy: map[v1.ResourceName]v1beta3.AutoscalingType{
						v1.ResourceCPU: v1beta3.AutoscalingTypeHorizontal,
					},
				},
			},
		},
	}

	tests := []struct {
		name                     string
		tortoise                 *v1beta3.Tortoise
		hpa                      *v2.HorizontalPodAutoscaler
		emergencyModeGracePeriod time.Duration
		expected                 bool
		description              string
	}{
		{
			name:     "Grace period active - recent failure should not trigger emergency mode",
			tortoise: horizontalTortoise,
			hpa: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.ScalingActive,
							Status:             "False",
							Reason:             "FailedGetResourceMetric",
							Message:            "failed to get cpu utilization: unable to fetch metrics from resource metrics API",
							LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Minute)), // Recent failure
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Name:      v1.ResourceCPU,
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			emergencyModeGracePeriod: defaultEmergencyModeGracePeriod, // Grace period longer than failure time
			expected:                 true,                            // Should NOT trigger emergency mode
			description:              "Recent failure within grace period should return true (no emergency)",
		},
		{
			name:     "Grace period expired - old failure should trigger emergency mode",
			tortoise: horizontalTortoise,
			hpa: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.ScalingActive,
							Status:             "False",
							Reason:             "FailedGetResourceMetric",
							Message:            "failed to get cpu utilization: unable to fetch metrics from resource metrics API",
							LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute)), // Old failure
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Name:      v1.ResourceCPU,
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			emergencyModeGracePeriod: defaultEmergencyModeGracePeriod, // Grace period shorter than failure time
			expected:                 false,                           // Should trigger emergency mode
			description:              "Old failure beyond grace period should return false (trigger emergency)",
		},
		{
			name:     "Short grace period - should trigger emergency mode quickly",
			tortoise: horizontalTortoise,
			hpa: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.ScalingActive,
							Status:             "False",
							Reason:             "FailedGetResourceMetric",
							Message:            "failed to get cpu utilization",
							LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Minute)), // 2 minutes ago
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Name:      v1.ResourceCPU,
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			emergencyModeGracePeriod: 1 * time.Minute, // Short grace period
			expected:                 false,           // Should trigger emergency mode
			description:              "Short grace period should allow emergency mode to trigger sooner",
		},
		{
			name:     "No failure condition - should work normally",
			tortoise: horizontalTortoise,
			hpa: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.ScalingActive,
							Status:             "True", // Working normally
							Reason:             "ValidMetricFound",
							Message:            "the HPA was able to successfully calculate a replica count",
							LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Name:      v1.ResourceCPU,
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](50), // Normal utilization
									AverageValue:       resource.NewQuantity(100, resource.DecimalSI),
									Value:              resource.NewQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			emergencyModeGracePeriod: defaultEmergencyModeGracePeriod,
			expected:                 true, // Should work normally
			description:              "Normal HPA operation should not be affected by grace period",
		},
		{
			name:     "Multiple failure conditions - should respect grace period for all",
			tortoise: horizontalTortoise,
			hpa: &v2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hpa",
					Namespace: "default",
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.ScalingActive,
							Status:             "False",
							Reason:             "FailedGetResourceMetric",
							Message:            "failed to get cpu utilization",
							LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute)), // Old failure
						},
						{
							Type:               v2.AbleToScale,
							Status:             "False",
							Reason:             "FailedGetScale",
							Message:            "failed to get scale",
							LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Minute)), // Recent failure
						},
					},
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Container: "app",
								Name:      v1.ResourceCPU,
								Current: v2.MetricValueStatus{
									AverageUtilization: ptr.To[int32](0),
									AverageValue:       resource.NewQuantity(0, resource.DecimalSI),
									Value:              resource.NewQuantity(0, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			emergencyModeGracePeriod: defaultEmergencyModeGracePeriod,
			expected:                 false, // Should trigger emergency mode due to old condition
			description:              "With multiple conditions, should trigger emergency if any are beyond grace period",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create service with the specified grace period
			c, err := New(
				fake.NewClientBuilder().Build(),
				record.NewFakeRecorder(10),
				0.95, 90, 100,
				time.Hour,
				nil,
				100, 1000, 3,
				"",
				tt.emergencyModeGracePeriod,
				false,
				nil,
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			result := c.IsHpaMetricAvailable(context.Background(), tt.tortoise, tt.hpa)

			if result != tt.expected {
				t.Errorf("Test '%s' failed: %s\nExpected: %v, Got: %v",
					tt.name, tt.description, tt.expected, result)
			}
		})
	}
}
