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
				MinMaxReplicasRecommendationType:           "daily",
				TTLHoursOfMinMaxReplicasRecommendation:     24 * 30,
				MaxReplicasFactor:                          2.0,
				MinReplicasFactor:                          0.5,
				ReplicaReductionFactor:                     0.95,
				UpperTargetResourceUtilization:             90,
				MinimumMinReplicas:                         3,
				PreferredReplicaNumUpperLimit:              30,
				MaximumCPUCores:                            "10",
				MaximumMemoryBytes:                         "10Gi",
				TimeZone:                                   "Asia/Tokyo",
				TortoiseUpdateInterval:                     1 * time.Hour,
				TortoiseHPATargetUtilizationMaxIncrease:    10,
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
				MinMaxReplicasRecommendationType:           "weekly",
				TTLHoursOfMinMaxReplicasRecommendation:     24 * 30,
				MaxReplicasFactor:                          2.0,
				MinReplicasFactor:                          0.5,
				ReplicaReductionFactor:                     0.95,
				UpperTargetResourceUtilization:             90,
				MinimumMinReplicas:                         3,
				PreferredReplicaNumUpperLimit:              30,
				MaximumCPUCores:                            "10",
				MaximumMemoryBytes:                         "10Gi",
				TimeZone:                                   "Asia/Tokyo",
				TortoiseUpdateInterval:                     15 * time.Second,
				TortoiseHPATargetUtilizationMaxIncrease:    5,
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
				MinMaxReplicasRecommendationType:           "weekly",
				TTLHoursOfMinMaxReplicasRecommendation:     24 * 30,
				MaxReplicasFactor:                          2.0,
				MinReplicasFactor:                          0.5,
				ReplicaReductionFactor:                     0.95,
				UpperTargetResourceUtilization:             90,
				MinimumMinReplicas:                         3,
				PreferredReplicaNumUpperLimit:              30,
				MaximumCPUCores:                            "10",
				MaximumMemoryBytes:                         "10Gi",
				TimeZone:                                   "Asia/Tokyo",
				TortoiseUpdateInterval:                     15 * time.Second,
				TortoiseHPATargetUtilizationMaxIncrease:    5,
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
