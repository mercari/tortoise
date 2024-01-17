package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// RangeOfMinMaxReplicasRecommendationHours is the time (hours) range of minReplicas and maxReplicas recommendation (default: 1)
	RangeOfMinMaxReplicasRecommendationHours int `yaml:"RangeOfMinMaxReplicasRecommendationHours"`
	// GatheringDataPeriodType means how long do we gather data for minReplica/maxReplica or data from VPA. "daily" and "weekly" are only valid value. (default: weekly)
	// If "daily", tortoise will consider all workload behaves very similarly every day.
	// If your workload may behave differently on, for example, weekdays and weekends, set this to "weekly".
	GatheringDataPeriodType string `yaml:"GatheringDataPeriodType"`
	// MaxReplicasFactor is the factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)
	MaxReplicasFactor float64 `yaml:"MaxReplicasFactor"`
	// MinReplicasFactor is the factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)
	MinReplicasFactor float64 `yaml:"MinReplicasFactor"`
	// ReplicaReductionFactor is the factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)
	ReplicaReductionFactor float64 `yaml:"ReplicaReductionFactor"`
	// UpperTargetResourceUtilization is the max target utilization that tortoise can give to the HPA (default: 90)
	UpperTargetResourceUtilization int `yaml:"UpperTargetResourceUtilization"`
	// MinimumMinReplicas is the minimum minReplicas that tortoise can give to the HPA (default: 3)
	MinimumMinReplicas int `yaml:"MinimumMinReplicas"`
	// PreferredReplicaNumUpperLimit is the replica number which the tortoise tries to keep the replica number less than. As it says "preferred", the tortoise **tries** to keep the replicas number less than this, but the replica number may be more than this when other "required" rule will be violated by this limit. (default: 30)
	PreferredReplicaNumUpperLimit int `yaml:"PreferredReplicaNumUpperLimit"`
	// MaximumCPUCores is the maximum CPU cores that the tortoise can give to the container (default: 10)
	MaximumCPUCores string `yaml:"MaximumCPUCores"`
	// MaximumMemoryBytes is the maximum memory bytes that the tortoise can give to the container (default: 10Gi)
	MaximumMemoryBytes string `yaml:"MaximumMemoryBytes"`
	// MinimumCPUCores is the minimum CPU cores that the tortoise can give to the container (default: 50m)
	MinimumCPUCores string `yaml:"MinimumCPUCores"`
	// MinimumMemoryBytes is the minimum memory bytes that the tortoise can give to the container (default: 50Mi)
	MinimumMemoryBytes string `yaml:"MinimumMemoryBytes"`
	// TimeZone is the timezone used to record time in tortoise objects (default: Asia/Tokyo)
	TimeZone string `yaml:"TimeZone"`
	// TortoiseUpdateInterval is the interval of updating each tortoise (default: 15s)
	TortoiseUpdateInterval time.Duration `yaml:"TortoiseUpdateInterval"`
	// TortoiseHPATargetUtilizationMaxIncrease is the max increase of target utilization that tortoise can give to the HPA (default: 5)
	TortoiseHPATargetUtilizationMaxIncrease int `yaml:"TortoiseHPATargetUtilizationMaxIncrease"`
	// TortoiseHPATargetUtilizationUpdateInterval is the interval of increasing target utilization of each HPA. (default: 1h)
	TortoiseHPATargetUtilizationUpdateInterval time.Duration `yaml:"TortoiseHPATargetUtilizationUpdateInterval"`

	// TortoiseHPAMaximumMinReplica is the maximum minReplica that tortoise can give to the HPA (default: 10)
	TortoiseHPAMaximumMinReplica int32 `yaml:"TortoiseHPAMaximumMinReplica"`

	// TODO: the following fields should be removed after we stop depending on deployment.
	// So, we don't put them in the documentation.
	// IstioSidecarProxyDefaultCPU is the default CPU resource request of the istio sidecar proxy (default: 100m)
	IstioSidecarProxyDefaultCPU string `yaml:"IstioSidecarProxyDefaultCPU"`
	// IstioSidecarProxyDefaultMemory is the default Memory resource request of the istio sidecar proxy (default: 200Mi)
	IstioSidecarProxyDefaultMemory string `yaml:"IstioSidecarProxyDefaultMemory"`
}

// ParseConfig parses the config file (yaml) and returns Config.
func ParseConfig(path string) (*Config, error) {
	config := &Config{
		RangeOfMinMaxReplicasRecommendationHours:   1,
		GatheringDataPeriodType:                    "weekly",
		MaxReplicasFactor:                          2.0,
		MinReplicasFactor:                          0.5,
		ReplicaReductionFactor:                     0.95,
		UpperTargetResourceUtilization:             90,
		MinimumMinReplicas:                         3,
		PreferredReplicaNumUpperLimit:              30,
		MaximumCPUCores:                            "10",
		MinimumCPUCores:                            "50m",
		MaximumMemoryBytes:                         "10Gi",
		MinimumMemoryBytes:                         "50Mi",
		TimeZone:                                   "Asia/Tokyo",
		TortoiseUpdateInterval:                     15 * time.Second,
		TortoiseHPATargetUtilizationMaxIncrease:    5,
		TortoiseHPATargetUtilizationUpdateInterval: time.Hour,
		TortoiseHPAMaximumMinReplica:               10,
		IstioSidecarProxyDefaultCPU:                "100m",
		IstioSidecarProxyDefaultMemory:             "200Mi",
	}
	if path == "" {
		return config, nil
	}

	// read file from path
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(b, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	if err := validate(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

func validate(config *Config) error {
	if config.RangeOfMinMaxReplicasRecommendationHours > 24 || config.RangeOfMinMaxReplicasRecommendationHours < 1 {
		return fmt.Errorf("RangeOfMinMaxReplicasRecommendationHours should be between 1 and 24")
	}

	if config.GatheringDataPeriodType != "daily" && config.GatheringDataPeriodType != "weekly" {
		return fmt.Errorf("GatheringDataPeriodType should be either \"daily\" or \"weekly\"")
	}

	if config.TortoiseHPATargetUtilizationMaxIncrease > 100 || config.TortoiseHPATargetUtilizationMaxIncrease <= 0 {
		return fmt.Errorf("TortoiseHPATargetUtilizationMaxIncrease should be between 1 and 100")
	}

	return nil
}
