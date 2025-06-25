package config

import (
	"reflect"
	"testing"
	"time"

	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/utils/ptr"
)

func TestParseConfig(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{
			name: "config file",
			args: args{
				path: "./testdata/config.yaml",
			},
			want: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				MaxReplicasRecommendationMultiplier:      2.0,
				MinReplicasRecommendationMultiplier:      0.5,
				ReplicaReductionFactor:                   0.95,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				PreferredMaxReplicas:                     30,
				MinimumCPURequest:                        "50m",
				MinimumMemoryRequest:                     "50Mi",
				MinimumTargetResourceUtilization:         65,
				MaximumCPURequest:                        "10",
				MaximumMemoryRequest:                     "10Gi",
				MinimumCPULimit:                          "1",
				TimeZone:                                 "Asia/Tokyo",
				TortoiseUpdateInterval:                   1 * time.Hour,
				HPATargetUtilizationMaxIncrease:          10,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				HPATargetUtilizationUpdateInterval:       3 * time.Hour,
				IstioSidecarProxyDefaultCPU:              "100m",
				IstioSidecarProxyDefaultMemory:           "200Mi",
				MaxAllowedScalingDownRatio:               0.5,
				ServiceGroups:                            []ServiceGroup{},
				MaximumMaxReplicasPerService:             []MaximumMaxReplicasPerGroup{},
				MinimumCPURequestPerContainer: map[string]string{
					"istio-proxy": "100m",
					"hoge-agent":  "120m",
				},
				MinimumMemoryRequestPerContainer: map[string]string{
					"istio-proxy": "1Mi",
					"hoge-agent":  "2Mi",
				},
				ResourceLimitMultiplier: map[string]int64{
					"cpu":    3,
					"memory": 1,
				},
				BufferRatioOnVerticalResource: 0.2,
			},
		},
		{
			name: "config file which has only one field",
			args: args{
				path: "./testdata/config-partly-override.yaml",
			},
			want: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 6,
				GatheringDataPeriodType:                  "weekly",
				MaxReplicasRecommendationMultiplier:      2.0,
				MinReplicasRecommendationMultiplier:      0.5,
				ReplicaReductionFactor:                   0.95,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MinimumTargetResourceUtilization:         65,
				PreferredMaxReplicas:                     30,
				MinimumCPURequest:                        "50m",
				MinimumMemoryRequest:                     "50Mi",
				MaximumCPURequest:                        "10",
				MaximumMemoryRequest:                     "10Gi",
				MinimumCPULimit:                          "0",
				TimeZone:                                 "Asia/Tokyo",
				TortoiseUpdateInterval:                   15 * time.Second,
				HPATargetUtilizationMaxIncrease:          5,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				HPATargetUtilizationUpdateInterval:       24 * time.Hour,
				IstioSidecarProxyDefaultCPU:              "100m",
				IstioSidecarProxyDefaultMemory:           "200Mi",
				MaxAllowedScalingDownRatio:               0.8,
				MinimumCPURequestPerContainer:            map[string]string{},
				MinimumMemoryRequestPerContainer:         map[string]string{},
				ResourceLimitMultiplier:                  map[string]int64{},
				ServiceGroups:                            []ServiceGroup{},
				MaximumMaxReplicasPerService:             []MaximumMaxReplicasPerGroup{},
				BufferRatioOnVerticalResource:            0.1,
			},
		},
		{
			name: "config file not found",
			args: args{
				path: "./testdata/not-found.yaml",
			},
			wantErr: true,
		},
		{
			name: "config file is empty",
			args: args{
				path: "",
			},
			want: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				MaxReplicasRecommendationMultiplier:      2.0,
				MinReplicasRecommendationMultiplier:      0.5,
				ReplicaReductionFactor:                   0.95,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				PreferredMaxReplicas:                     30,
				MinimumCPURequest:                        "50m",
				MinimumMemoryRequest:                     "50Mi",
				MinimumTargetResourceUtilization:         65,
				MaximumCPURequest:                        "10",
				MaximumMemoryRequest:                     "10Gi",
				MinimumCPULimit:                          "0",
				TimeZone:                                 "Asia/Tokyo",
				TortoiseUpdateInterval:                   15 * time.Second,
				HPATargetUtilizationMaxIncrease:          5,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				HPATargetUtilizationUpdateInterval:       24 * time.Hour,
				IstioSidecarProxyDefaultCPU:              "100m",
				IstioSidecarProxyDefaultMemory:           "200Mi",
				MaxAllowedScalingDownRatio:               0.8,
				MinimumCPURequestPerContainer:            map[string]string{},
				MinimumMemoryRequestPerContainer:         map[string]string{},
				ResourceLimitMultiplier:                  map[string]int64{},
				ServiceGroups:                            []ServiceGroup{},
				MaximumMaxReplicasPerService:             []MaximumMaxReplicasPerGroup{},
				BufferRatioOnVerticalResource:            0.1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:   "default config is valid",
			config: defaultConfig(),
		},
		{
			name: "invalid RangeOfMinMaxReplicasRecommendationHours",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 25,
			},
			wantErr: true,
		},
		{
			name: "invalid GatheringDataPeriodType",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid HPATargetUtilizationMaxIncrease",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				HPATargetUtilizationMaxIncrease:          101,
			},
			wantErr: true,
		},
		{
			name: "invalid MinimumMinReplicas",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				HPATargetUtilizationMaxIncrease:          99,
				MinimumMinReplicas:                       10,
				MaximumMinReplicas:                       1,
			},
			wantErr: true,
		},
		{
			name: "invalid MaximumMinReplicas",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				HPATargetUtilizationMaxIncrease:          99,
				MinimumMinReplicas:                       2,
				MaximumMinReplicas:                       20,
				MaximumMaxReplicas:                       10,
			},
			wantErr: true,
		},
		{
			name: "invalid PreferredMaxReplicas",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				HPATargetUtilizationMaxIncrease:          99,
				MinimumMinReplicas:                       2,
				MaximumMinReplicas:                       20,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     101,
			},
			wantErr: true,
		},
		{
			name: "invalid PreferredMaxReplicas less than MinimumMinReplicas",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				HPATargetUtilizationMaxIncrease:          99,
				MinimumMinReplicas:                       5,
				MaximumMinReplicas:                       20,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     4,
			},
			wantErr: true,
		},
		{
			name: "invalid MaxAllowedScalingDownRatio",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				HPATargetUtilizationMaxIncrease:          99,
				MinimumMinReplicas:                       5,
				MaximumMinReplicas:                       20,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     6,
				MaxAllowedScalingDownRatio:               1.1,
			},
			wantErr: true,
		},
		{
			name: "invalid ResourceLimitMultiplier",
			config: &Config{
				ResourceLimitMultiplier: map[string]int64{
					"cpu":    0,
					"memory": 1,
				},
			},
			wantErr: true,
		},
		{
			name: "valid HPA behavior - nil behavior",
			config: &Config{
				DefaultHPABehavior:                       nil,
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: false,
		},
		{
			name: "valid HPA behavior - empty policies",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp:   &v2.HPAScalingRules{},
					ScaleDown: &v2.HPAScalingRules{},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: false,
		},
		{
			name: "valid HPA behavior - only scale up policy",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         100,
								PeriodSeconds: 60,
							},
						},
						StabilizationWindowSeconds: ptr.To[int32](0),
						SelectPolicy:               ptr.To(v2.ScalingPolicySelect("Max")),
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: false,
		},
		{
			name: "valid HPA behavior - only scale down policy",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         2,
								PeriodSeconds: 90,
							},
						},
						StabilizationWindowSeconds: ptr.To[int32](300),
						SelectPolicy:               ptr.To(v2.ScalingPolicySelect("Min")),
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: false,
		},
		{
			name: "valid HPA behavior - comprehensive scale up and down policies",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         100,
								PeriodSeconds: 60,
							},
							{
								Type:          v2.PodsScalingPolicy,
								Value:         4,
								PeriodSeconds: 60,
							},
						},
						StabilizationWindowSeconds: ptr.To[int32](0),
						SelectPolicy:               ptr.To(v2.ScalingPolicySelect("Max")),
					},
					ScaleDown: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         2,
								PeriodSeconds: 90,
							},
							{
								Type:          v2.PodsScalingPolicy,
								Value:         1,
								PeriodSeconds: 60,
							},
						},
						StabilizationWindowSeconds: ptr.To[int32](300),
						SelectPolicy:               ptr.To(v2.ScalingPolicySelect("Min")),
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: false,
		},
		{
			name: "invalid HPA behavior - invalid policy type",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          "InvalidType",
								Value:         100,
								PeriodSeconds: 60,
							},
						},
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid HPA behavior - invalid policy value",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         0,
								PeriodSeconds: 60,
							},
						},
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid HPA behavior - invalid period seconds",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         100,
								PeriodSeconds: 2000, // > 1800
							},
						},
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid HPA behavior - invalid stabilization window seconds",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         100,
								PeriodSeconds: 60,
							},
						},
						StabilizationWindowSeconds: ptr.To[int32](4000), // > 3600
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid HPA behavior - invalid select policy",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          "InvalidType",
								Value:         100,
								PeriodSeconds: 60,
							},
						},
						SelectPolicy: ptr.To(v2.ScalingPolicySelect("InvalidPolicy")),
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid HPA behavior - negative stabilization window seconds",
			config: &Config{
				DefaultHPABehavior: &v2.HorizontalPodAutoscalerBehavior{
					ScaleUp: &v2.HPAScalingRules{
						Policies: []v2.HPAScalingPolicy{
							{
								Type:          v2.PercentScalingPolicy,
								Value:         100,
								PeriodSeconds: 60,
							},
						},
						StabilizationWindowSeconds: ptr.To[int32](-1),
					},
				},
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
			},
			wantErr: true,
		},
		// ServiceGroup validation test cases
		{
			name: "invalid ServiceGroup - empty service group name",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{
						Name: "", // Empty name should be invalid
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid ServiceGroup - duplicate service group names",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
					{Name: "frontend"}, // Duplicate name should be invalid
				},
			},
			wantErr: true,
		},
		// MaximumMaxReplicasPerGroup validation test cases
		{
			name: "invalid MaximumMaxReplicasPerService - empty ServiceGroupName",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
				},
				MaximumMaxReplicasPerService: []MaximumMaxReplicasPerGroup{
					{
						ServiceGroupName:  "", // Empty ServiceGroupName should be invalid
						MaximumMaxReplica: 50,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid MaximumMaxReplicasPerService - ServiceGroupName not defined in ServiceGroups",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
				},
				MaximumMaxReplicasPerService: []MaximumMaxReplicasPerGroup{
					{
						ServiceGroupName:  "backend", // Not defined in ServiceGroups
						MaximumMaxReplica: 50,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid MaximumMaxReplicasPerService - negative MaximumMaxReplica",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
				},
				MaximumMaxReplicasPerService: []MaximumMaxReplicasPerGroup{
					{
						ServiceGroupName:  "frontend",
						MaximumMaxReplica: -5, // Negative value should be invalid
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid MaximumMinReplicas greater than service group MaximumMaxReplica",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       15, // Greater than service group MaximumMaxReplica
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
				},
				MaximumMaxReplicasPerService: []MaximumMaxReplicasPerGroup{
					{
						ServiceGroupName:  "frontend",
						MaximumMaxReplica: 10, // Less than MaximumMinReplicas
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid PreferredMaxReplicas greater than service group MaximumMaxReplica",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     25, // Greater than service group MaximumMaxReplica
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
				},
				MaximumMaxReplicasPerService: []MaximumMaxReplicasPerGroup{
					{
						ServiceGroupName:  "frontend",
						MaximumMaxReplica: 20, // Less than PreferredMaxReplicas
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid ServiceGroups and MaximumMaxReplicasPerService configuration",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 1,
				GatheringDataPeriodType:                  "weekly",
				HPATargetUtilizationMaxIncrease:          5,
				MinimumTargetResourceUtilization:         65,
				MaximumTargetResourceUtilization:         90,
				MinimumMinReplicas:                       3,
				MaximumMinReplicas:                       10,
				MaximumMaxReplicas:                       100,
				PreferredMaxReplicas:                     30,
				MaxAllowedScalingDownRatio:               0.8,
				ServiceGroups: []ServiceGroup{
					{Name: "frontend"},
					{Name: "backend"},
				},
				MaximumMaxReplicasPerService: []MaximumMaxReplicasPerGroup{
					{
						ServiceGroupName:  "frontend",
						MaximumMaxReplica: 50,
					},
					{
						ServiceGroupName:  "backend",
						MaximumMaxReplica: 80,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.config); (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
