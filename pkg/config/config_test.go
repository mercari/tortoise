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
				RangeOfMinMaxReplicasRecommendationHours:   2,
				GatheringDataPeriodType:                    "daily",
				MaxReplicasFactor:                          2.0,
				MinReplicasFactor:                          0.5,
				ReplicaReductionFactor:                     0.95,
				UpperTargetResourceUtilization:             90,
				MinimumMinReplicas:                         3,
				PreferredReplicaNumUpperLimit:              30,
				MinimumCPUCores:                            "50m",
				MinimumMemoryBytes:                         "50Mi",
				MinimumTargetResourceUtilization:           65,
				MaximumCPUCores:                            "10",
				MaximumMemoryBytes:                         "10Gi",
				TimeZone:                                   "Asia/Tokyo",
				TortoiseUpdateInterval:                     1 * time.Hour,
				TortoiseHPATargetUtilizationMaxIncrease:    10,
				MaximumMinReplica:                          10,
				MaximumMaxReplica:                          100,
				TortoiseHPATargetUtilizationUpdateInterval: 3 * time.Hour,
				IstioSidecarProxyDefaultCPU:                "100m",
				IstioSidecarProxyDefaultMemory:             "200Mi",
			},
		},
		{
			name: "config file which has only one field",
			args: args{
				path: "./testdata/config-partly-override.yaml",
			},
			want: &Config{
				RangeOfMinMaxReplicasRecommendationHours:   6,
				GatheringDataPeriodType:                    "weekly",
				MaxReplicasFactor:                          2.0,
				MinReplicasFactor:                          0.5,
				ReplicaReductionFactor:                     0.95,
				UpperTargetResourceUtilization:             90,
				MinimumMinReplicas:                         3,
				MinimumTargetResourceUtilization:           65,
				PreferredReplicaNumUpperLimit:              30,
				MinimumCPUCores:                            "50m",
				MinimumMemoryBytes:                         "50Mi",
				MaximumCPUCores:                            "10",
				MaximumMemoryBytes:                         "10Gi",
				TimeZone:                                   "Asia/Tokyo",
				TortoiseUpdateInterval:                     15 * time.Second,
				TortoiseHPATargetUtilizationMaxIncrease:    5,
				MaximumMinReplica:                          10,
				MaximumMaxReplica:                          100,
				TortoiseHPATargetUtilizationUpdateInterval: 1 * time.Hour,
				IstioSidecarProxyDefaultCPU:                "100m",
				IstioSidecarProxyDefaultMemory:             "200Mi",
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
				RangeOfMinMaxReplicasRecommendationHours:   1,
				GatheringDataPeriodType:                    "weekly",
				MaxReplicasFactor:                          2.0,
				MinReplicasFactor:                          0.5,
				ReplicaReductionFactor:                     0.95,
				UpperTargetResourceUtilization:             90,
				MinimumMinReplicas:                         3,
				PreferredReplicaNumUpperLimit:              30,
				MinimumCPUCores:                            "50m",
				MinimumMemoryBytes:                         "50Mi",
				MinimumTargetResourceUtilization:           65,
				MaximumCPUCores:                            "10",
				MaximumMemoryBytes:                         "10Gi",
				TimeZone:                                   "Asia/Tokyo",
				TortoiseUpdateInterval:                     15 * time.Second,
				TortoiseHPATargetUtilizationMaxIncrease:    5,
				MaximumMinReplica:                          10,
				MaximumMaxReplica:                          100,
				TortoiseHPATargetUtilizationUpdateInterval: time.Hour,
				IstioSidecarProxyDefaultCPU:                "100m",
				IstioSidecarProxyDefaultMemory:             "200Mi",
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
			name: "invalid TortoiseHPATargetUtilizationMaxIncrease",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				TortoiseHPATargetUtilizationMaxIncrease:  101,
			},
			wantErr: true,
		},
		{
			name: "invalid MinimumMinReplicas",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				TortoiseHPATargetUtilizationMaxIncrease:  99,
				MinimumMinReplicas:                       10,
				MaximumMinReplica:                        1,
			},
			wantErr: true,
		},
		{
			name: "invalid MaximumMinReplica",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				TortoiseHPATargetUtilizationMaxIncrease:  99,
				MinimumMinReplicas:                       2,
				MaximumMinReplica:                        20,
				MaximumMaxReplica:                        10,
			},
			wantErr: true,
		},
		{
			name: "invalid PreferredReplicaNumUpperLimit",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				TortoiseHPATargetUtilizationMaxIncrease:  99,
				MinimumMinReplicas:                       2,
				MaximumMinReplica:                        20,
				MaximumMaxReplica:                        100,
				PreferredReplicaNumUpperLimit:            101,
			},
			wantErr: true,
		},
		{
			name: "invalid PreferredReplicaNumUpperLimit",
			config: &Config{
				RangeOfMinMaxReplicasRecommendationHours: 2,
				GatheringDataPeriodType:                  "daily",
				TortoiseHPATargetUtilizationMaxIncrease:  99,
				MinimumMinReplicas:                       5,
				MaximumMinReplica:                        20,
				MaximumMaxReplica:                        100,
				PreferredReplicaNumUpperLimit:            4,
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
