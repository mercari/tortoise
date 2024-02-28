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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
)

func TestClient_UpdateHPAFromTortoiseRecommendation(t *testing.T) {
	now := metav1.NewTime(time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC))

	type args struct {
		ctx      context.Context
		tortoise *autoscalingv1beta3.Tortoise
		now      time.Time
	}
	tests := []struct {
		name               string
		args               args
		excludeMetricRegex string
		initialHPA         *v2.HorizontalPodAutoscaler
		want               *v2.HorizontalPodAutoscaler
		wantTortoise       *autoscalingv1beta3.Tortoise
		wantErr            bool
	}{
		{
			name: "Basic test case with container resource metrics",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Conditions: autoscalingv1beta3.Conditions{
							TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
								{
									Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					Conditions: autoscalingv1beta3.Conditions{
						TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
							{
								Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: autoscalingv1beta3.Recommendations{
						Horizontal: autoscalingv1beta3.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Conditions: autoscalingv1beta3.Conditions{
							TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
								{
									Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					Conditions: autoscalingv1beta3.Conditions{
						TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
							{
								Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: autoscalingv1beta3.Recommendations{
						Horizontal: autoscalingv1beta3.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Conditions: autoscalingv1beta3.Conditions{
							TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
								{
									Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     10000,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					Conditions: autoscalingv1beta3.Conditions{
						TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
							{
								Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: autoscalingv1beta3.Recommendations{
						Horizontal: autoscalingv1beta3.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     10000,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
			name: "maximum maxReplica is applied",
			args: args{
				ctx: context.Background(),
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Conditions: autoscalingv1beta3.Conditions{
							TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
								{
									Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-3 * time.Hour)),
									LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     999999,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					Conditions: autoscalingv1beta3.Conditions{
						TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
							{
								Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: autoscalingv1beta3.Recommendations{
						Horizontal: autoscalingv1beta3.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     999999,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Spec: autoscalingv1beta3.TortoiseSpec{
						UpdateMode: autoscalingv1beta3.UpdateModeAuto,
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Conditions: autoscalingv1beta3.Conditions{
							TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
								{
									Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
									Status:             v1.ConditionTrue,
									LastUpdateTime:     metav1.NewTime(now.Add(-1 * time.Minute)),
									LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
									Reason:             "HPATargetUtilizationUpdated",
									Message:            "HPA target utilization is updated",
								},
							},
						},
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				Spec: autoscalingv1beta3.TortoiseSpec{
					UpdateMode: autoscalingv1beta3.UpdateModeAuto,
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					Conditions: autoscalingv1beta3.Conditions{
						TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
							{
								Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					Recommendations: autoscalingv1beta3.Recommendations{
						Horizontal: autoscalingv1beta3.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Spec: autoscalingv1beta3.TortoiseSpec{
						UpdateMode: autoscalingv1beta3.UpdateModeOff,
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Spec: autoscalingv1beta3.TortoiseSpec{
						UpdateMode: autoscalingv1beta3.UpdateModeAuto,
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						TortoisePhase: autoscalingv1beta3.TortoisePhaseEmergency,
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						TortoisePhase: autoscalingv1beta3.TortoisePhaseBackToNormal,
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
				tortoise: &autoscalingv1beta3.Tortoise{
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						Targets: autoscalingv1beta3.TargetsStatus{
							HorizontalPodAutoscaler: "hpa",
						},
						TortoisePhase: autoscalingv1beta3.TortoisePhaseBackToNormal,
						Recommendations: autoscalingv1beta3.Recommendations{
							Horizontal: autoscalingv1beta3.HorizontalRecommendations{
								TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
								MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
									{
										From:      0,
										To:        2,
										Value:     6,
										UpdatedAt: now,
										WeekDay:   ptr.To(now.Weekday().String()),
									},
								},
								MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
					Behavior:    globalRecommendedHPABehavior.DeepCopy(),
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					Conditions: autoscalingv1beta3.Conditions{
						TortoiseConditions: []autoscalingv1beta3.TortoiseCondition{
							{
								Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
								Status:             v1.ConditionTrue,
								LastUpdateTime:     now,
								LastTransitionTime: now,
								Reason:             "HPATargetUtilizationUpdated",
								Message:            "HPA target utilization is updated",
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
					TortoisePhase: autoscalingv1beta3.TortoisePhaseWorking,
					Recommendations: autoscalingv1beta3.Recommendations{
						Horizontal: autoscalingv1beta3.HorizontalRecommendations{
							TargetUtilizations: []autoscalingv1beta3.HPATargetUtilizationRecommendationPerContainer{
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
							MaxReplicas: []autoscalingv1beta3.ReplicasRecommendation{
								{
									From:      0,
									To:        2,
									Value:     6,
									UpdatedAt: now,
									WeekDay:   ptr.To(now.Weekday().String()),
								},
							},
							MinReplicas: []autoscalingv1beta3.ReplicasRecommendation{
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
			c, err := New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90, 50, time.Hour, 1000, 10001, tt.excludeMetricRegex)
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

func TestService_InitializeHPA(t *testing.T) {
	type args struct {
		tortoise   *autoscalingv1beta3.Tortoise
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind:       "Deployment",
								Name:       "deployment",
								APIVersion: "apps/v1",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					MinReplicas: ptrInt32(2),
					MaxReplicas: 8,
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(fake.NewClientBuilder().Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, 100, 1000, "")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if tt.initialHPA != nil {
				c, err = New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, 100, 1000, "")
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
		tortoise   *autoscalingv1beta3.Tortoise
		replicaNum int32
	}
	tests := []struct {
		name string
		// initialHPA is the initial state of the HPA in kube-apiserver
		initialHPA   *v2.HorizontalPodAutoscaler
		args         args
		afterHPA     *v2.HorizontalPodAutoscaler
		wantTortoise *autoscalingv1beta3.Tortoise
		wantErr      bool
	}{
		{
			name: "add metrics to existing hpa",
			args: args{
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					TargetRefs: autoscalingv1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					TargetRefs: autoscalingv1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						DeletionPolicy: autoscalingv1beta3.DeletionPolicyDeleteAll,
						TargetRefs: autoscalingv1beta3.TargetRefs{
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					DeletionPolicy: autoscalingv1beta3.DeletionPolicyDeleteAll,
					TargetRefs: autoscalingv1beta3.TargetRefs{
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
					},
				},
			},
			afterHPA: nil, // hpa would be removed
		},
		{
			name: "when remove all horizontal, hpa would be disabled if it's created by users",
			args: args{
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						DeletionPolicy: autoscalingv1beta3.DeletionPolicyDeleteAll,
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					DeletionPolicy: autoscalingv1beta3.DeletionPolicyDeleteAll,
					TargetRefs: autoscalingv1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					TargetRefs: autoscalingv1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					TargetRefs: autoscalingv1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseGatheringData,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
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
				tortoise: &autoscalingv1beta3.Tortoise{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tortoise",
						Namespace: "default",
					},
					Spec: autoscalingv1beta3.TortoiseSpec{
						TargetRefs: autoscalingv1beta3.TargetRefs{
							HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
							ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
								Kind: "Deployment",
								Name: "deployment",
							},
						},
					},
					Status: autoscalingv1beta3.TortoiseStatus{
						AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
						ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
							{
								ContainerName: "app",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
							{
								ContainerName: "istio-proxy",
								ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
									v1.ResourceCPU: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
									v1.ResourceMemory: {
										Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
									},
								},
							},
						},
						Targets: autoscalingv1beta3.TargetsStatus{
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
			wantTortoise: &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tortoise",
					Namespace: "default",
				},
				Spec: autoscalingv1beta3.TortoiseSpec{
					TargetRefs: autoscalingv1beta3.TargetRefs{
						HorizontalPodAutoscalerName: ptr.To("existing-hpa"),
						ScaleTargetRef: autoscalingv1beta3.CrossVersionObjectReference{
							Kind: "Deployment",
							Name: "deployment",
						},
					},
				},
				Status: autoscalingv1beta3.TortoiseStatus{
					AutoscalingPolicy: []autoscalingv1beta3.ContainerAutoscalingPolicy{
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
					ContainerResourcePhases: []autoscalingv1beta3.ContainerResourcePhases{
						{
							ContainerName: "app",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
						{
							ContainerName: "istio-proxy",
							ResourcePhases: map[v1.ResourceName]autoscalingv1beta3.ResourcePhase{
								v1.ResourceCPU: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
								v1.ResourceMemory: {
									Phase: autoscalingv1beta3.ContainerResourcePhaseWorking,
								},
							},
						},
					},
					Targets: autoscalingv1beta3.TargetsStatus{
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
			c, err := New(fake.NewClientBuilder().Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, 1000, 10000, "")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if tt.initialHPA != nil {
				c, err = New(fake.NewClientBuilder().WithRuntimeObjects(tt.initialHPA).Build(), record.NewFakeRecorder(10), 0.95, 90, 100, time.Hour, 1000, 10000, "")
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
