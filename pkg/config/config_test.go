package config

import (
	"reflect"
	"testing"
	"time"
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
			name: "invalid PreferredMaxReplicas",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.config); (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
