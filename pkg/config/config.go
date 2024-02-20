package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mercari/tortoise/pkg/features"
)

type Config struct {
	// RangeOfMinMaxReplicasRecommendationHours is the time (hours) range of minReplicas and maxReplicas recommendation (default: 1)
	//
	//```yaml
	// kind: Tortoise
	// #...
	// status:
	//   recommendations:
	//     horizontal:
	//       minReplicas:
	//         - from: 0
	//           to: 1
	//           weekday: Sunday
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	//         - from: 1
	//           to: 2
	//           weekday: Sunday
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	// ```
	RangeOfMinMaxReplicasRecommendationHours int `yaml:"RangeOfMinMaxReplicasRecommendationHours"`
	// GatheringDataPeriodType means how long do we gather data for minReplica/maxReplica or data from VPA. "daily" and "weekly" are only valid value. (default: weekly)
	// If "daily", tortoise will consider all workload behaves very similarly every day.
	// If your workload may behave differently on, for example, weekdays and weekends, set this to "weekly".
	//
	// **daily**
	//
	// ```yaml
	// kind: Tortoise
	// #...
	// status:
	//   recommendations:
	//     horizontal:
	//       minReplicas:
	//         # This recommendation is from 0am to 1am on all days of week.
	//         - from: 0
	//           to: 1
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	//         - from: 1
	//           to: 2
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	//         # ...
	//         - from: 23
	//           to: 24
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	// ```
	//
	// **weekly**
	//
	// ```yaml
	// kind: Tortoise
	// #...
	// status:
	//   recommendations:
	//     horizontal:
	//       minReplicas:
	//         # This recommendation is from 0am to 1am on Sundays.
	//         - from: 0
	//           to: 1
	//           weekday: Sunday # Recommendation is generated for each day of week.
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	//         - from: 1
	//           to: 2
	//           weekday: Sunday
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	//         # ...
	//         - from: 23
	//           to: 24
	//           weekday: Saturday
	//           timezone: Asia/Tokyo
	//           value: 3
	//           updatedAt: 2023-01-01T00:00:00Z
	// ```
	GatheringDataPeriodType string `yaml:"GatheringDataPeriodType"`
	// MaxReplicasFactor is the factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)
	// If the current replica number is 15 and `MaxReplicasFactor` is 2.0,
	// the maxReplicas recommendation from the current situation will be 30 (15 * 2.0).
	//
	// ```yaml
	// kind: Tortoise
	// #...
	// status:
	//   recommendations:
	//     horizontal:
	//       maxReplicas:
	//         - from: 0
	//           to: 1
	//           weekday: Sunday
	//           timezone: Asia/Tokyo
	//           value: 30
	//           updatedAt: 2023-01-01T00:00:00Z
	// ```
	MaxReplicasFactor float64 `yaml:"MaxReplicasFactor"`
	// MinReplicasFactor is the factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)
	// If the current replica number is 10 and `MaxReplicasFactor` is 0.5,
	// the minReplicas recommendation from the current situation will be 5 (10 * 0.5).
	//
	// ```yaml
	// kind: Tortoise
	// #...
	// status:
	//   recommendations:
	//     horizontal:
	//       minReplicas:
	//         - from: 0
	//           to: 1
	//           weekday: Sunday
	//           timezone: Asia/Tokyo
	//           value: 5
	//           updatedAt: 2023-01-01T00:00:00Z
	// ```
	MinReplicasFactor float64 `yaml:"MinReplicasFactor"`
	// ReplicaReductionFactor is the factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)
	//
	// Let's say `ReplicaReductionFactor` is 0.95,
	// the minReplicas was increased to 100 due to the emergency mode,
	// and a user just turned off the emergency mode now.
	//
	// Then, the `minReplicas` is going to change like:
	//
	// 100 --(*0.95)--> 95 --(*0.95)--> 91 -- ...
	//
	// It's reduced every time tortoise is evaluated by the controller. (= once a `TortoiseUpdateInterval`)
	ReplicaReductionFactor float64 `yaml:"ReplicaReductionFactor"`
	// MaximumTargetResourceUtilization is the max target utilization that tortoise can give to the HPA (default: 90)
	MaximumTargetResourceUtilization int `yaml:"MaximumTargetResourceUtilization"`
	// MinimumTargetResourceUtilization is the min target utilization that tortoise can give to the HPA (default: 65)
	MinimumTargetResourceUtilization int `yaml:"MinimumTargetResourceUtilization"`
	// MinimumMinReplicas is the minimum minReplicas that tortoise can give to the HPA (default: 3)
	MinimumMinReplicas int `yaml:"MinimumMinReplicas"`
	// MaximumMinReplicas is the maximum minReplica that tortoise can give to the HPA (default: 10)
	MaximumMinReplicas int32 `yaml:"MaximumMinReplicas"`
	// PreferredMaxReplicas is the replica number which the tortoise tries to keep the replica number less than.
	// As it says "preferred", the tortoise **tries** to keep the replicas number less than this,
	// but the replica number may be more than this when other "required" rule will be violated by this limit. (default: 30)
	//
	// When the number of replicas reaches `PreferredMaxReplicas`,
	// a tortoise will increase the Pod's resource request instead of increasing the number of replicas.
	//
	// But, when the resource request reaches `MaximumCPUCores` or `MaximumMemoryBytes`,
	// a tortoise will ignore `PreferredMaxReplicas`, and increase the number of replicas.
	// This feature is controlled by the feature flag `VerticalScalingBasedOnPreferredMaxReplicas`.
	PreferredMaxReplicas int `yaml:"PreferredMaxReplicas"`
	// MaximumMaxReplicas is the maximum maxReplica that tortoise can give to the HPA (default: 100)
	// Note that this is very dangerous. If you set this value too low, the HPA may not be able to scale up the workload.
	// The motivation is to use it has a hard limit to prevent the HPA from scaling up the workload too much in cases of Tortoise's bug, abnormal traffic increase, etc.
	// If some Tortoise hits this limit, the tortoise controller emits an error log, which may or may not imply you have to change this value.
	MaximumMaxReplicas int32 `yaml:"MaximumMaxReplicas"`
	// MaximumCPUCores is the maximum CPU cores that the tortoise can give to the container resource request (default: 10)
	MaximumCPUCores string `yaml:"MaximumCPUCores"`
	// MaximumMemoryBytes is the maximum memory bytes that the tortoise can give to the container resource request (default: 10Gi)
	MaximumMemoryBytes string `yaml:"MaximumMemoryBytes"`
	// MinimumCPUCores is the minimum CPU cores that the tortoise can give to the container resource request (default: 50m)
	MinimumCPUCores string `yaml:"MinimumCPUCores"`
	// MinimumCPULimitCores is the minimum CPU cores that the tortoise can give to the container resource limit (default: 0)
	// Note that this configuration is prioritized over ResourceLimitMultiplier.
	//
	// e.g., if you set `MinimumCPULimitCores: 100m` and `ResourceLimitMultiplier: cpu: 3`, and the container requests 10m CPU,
	// Tortoise will set the limit to 100m, not 30m.
	MinimumCPULimitCores string `yaml:"MinimumCPULimitCores"`
	// MinimumCPUCoresPerContainer is the minimum CPU cores per container that the tortoise can give to the container resource request (default: nil)
	// It has a higher priority than MinimumCPUCores.
	// If you specify both, the tortoise uses MinimumCPUCoresPerContainer basically, but if the container name is not found in this map, the tortoise uses MinimumCPUCores.
	//
	// You can specify like this:
	// ```
	// MinimumCPUCoresPerContainer:
	//  istio-proxy: 100m
	//  hoge-agent: 120m
	// ```
	MinimumCPUCoresPerContainer map[string]string `yaml:"MinimumCPUCoresPerContainer"`
	// MinimumMemoryBytes is the minimum memory bytes that the tortoise can give to the container resource request (default: 50Mi)
	MinimumMemoryBytes string `yaml:"MinimumMemoryBytes"`
	// MinimumMemoryBytesPerContainer is the minimum memory bytes per container that the tortoise can give to the container (default: nil)
	// If you specify both, the tortoise uses MinimumMemoryBytesPerContainer basically, but if the container name is not found in this map, the tortoise uses MinimumMemoryBytes.
	//
	// You can specify like this:
	// ```
	// MinimumMemoryBytesPerContainer:
	//  istio-proxy: 100m
	//  hoge-agent: 120m
	// ```
	MinimumMemoryBytesPerContainer map[string]string `yaml:"MinimumMemoryBytesPerContainer"`
	// TimeZone is the timezone used to record time in tortoise objects (default: Asia/Tokyo)
	TimeZone string `yaml:"TimeZone"`
	// TortoiseUpdateInterval is the interval of updating each tortoise (default: 15s)
	// (It may delay if there are many tortoise objects in the cluster.)
	TortoiseUpdateInterval time.Duration `yaml:"TortoiseUpdateInterval"`
	// HPATargetUtilizationMaxIncrease is the max increase of target utilization that tortoise can give to the HPA (default: 5)
	// If tortoise suggests changing the HPA target resource utilization from 50 to 80, it might be dangerous to give the change at once.
	// By configuring this, we can limit the max increase that tortoise can make.
	// So, if HPATargetUtilizationMaxIncrease is 5, even if tortoise suggests changing the HPA target resource utilization from 50 to 80,
	// the target utilization is actually change from 50 to 55.
	HPATargetUtilizationMaxIncrease int `yaml:"HPATargetUtilizationMaxIncrease"`
	// HPATargetUtilizationUpdateInterval is the interval of updating target utilization of each HPA (default: 24h)
	//
	// So, similarily to HPATargetUtilizationMaxIncrease, it's also a safety guard to prevent HPA target utilization from suddenly changed.
	// If HPATargetUtilizationMaxIncrease is 1h, HPATargetUtilizationMaxIncrease is 5, and tortoise keep suggesting changing the HPA target resource utilization from 50 to 80,
	// the target resource utilization would be changing like 50 -(1h)-> 55 -(1h)-> 60 → ... → 80.
	HPATargetUtilizationUpdateInterval time.Duration `yaml:"HPATargetUtilizationUpdateInterval"`
	// HPAExternalMetricExclusionRegex is the regex to exclude external metrics from HPA. (default: Not delete any external metrics)
	// Basically, if HPA has external metrics, the tortoise keeps that external metric.
	// But, if you want to remove some external metrics from HPA, you can use this regex.
	// Note, the exclusion is done only when tortoise is not Off mode.
	// For example, if you set `datadogmetric.*` in `HPAExternalMetricExclusionRegex`,
	// all the external metric which name matches `datadogmetric.*` regex are removed by Tortoise once Tortoise is in Auto mode.
	HPAExternalMetricExclusionRegex string `yaml:"HPAExternalMetricExclusionRegex"`

	// MaxAllowedVerticalScalingDownRatio is the max allowed scaling down ratio (default: 0.8)
	// For example, if the current resource request is 100m, the max allowed scaling down ratio is 0.8,
	// the minimum resource request that Tortoise can apply is 80m.
	MaxAllowedScalingDownRatio float64 `yaml:"MaxAllowedScalingDownRatio"`

	// ResourceLimitMultiplier is the multiplier to calculate the resource limit from the resource request (default: nil)
	// (The key is the resource name, and the value is the multiplier.)
	//
	// VPA changes the resource limit based on the resource request; it maintains limit to request ratio specified for all containers.
	// Meaning, users have to configure the resource limit properly based on the resource request before adopting Tortoise
	// so that VPA can adjust the resource limit properly.
	// This feature is to remove the responsibility from the user to configure the resource limit and let Tortoise manage the resource limit fully.
	// For example, if you set ResourceLimitMultiplier 3 and Pod's resource request is 100m, the limit will be changed to 300m,
	// regardless of which resource limit is set in the Pod originally.
	// Also, see MinimumCPULimitCores and MinimumMemoryLimitBytes.
	//
	// The default value is nil; Tortoise doesn't change the resource limit itself.
	ResourceLimitMultiplier map[string]int64 `yaml:"ResourceLimitMultiplier"`

	// TODO: the following fields should be removed after we stop depending on deployment.
	// So, we don't put them in the documentation.
	// IstioSidecarProxyDefaultCPU is the default CPU resource request of the istio sidecar proxy (default: 100m)
	IstioSidecarProxyDefaultCPU string `yaml:"IstioSidecarProxyDefaultCPU"`
	// IstioSidecarProxyDefaultMemory is the default Memory resource request of the istio sidecar proxy (default: 200Mi)
	IstioSidecarProxyDefaultMemory string `yaml:"IstioSidecarProxyDefaultMemory"`

	// FeatureFlags is the list of feature flags (default: empty = all alpha features are disabled)
	// See the list of feature flags in features.go
	FeatureFlags []features.FeatureFlag `yaml:"FeatureFlags"`
}

func defaultConfig() *Config {
	return &Config{
		RangeOfMinMaxReplicasRecommendationHours: 1,
		GatheringDataPeriodType:                  "weekly",
		MaxReplicasFactor:                        2.0,
		MinReplicasFactor:                        0.5,
		ReplicaReductionFactor:                   0.95,
		MinimumTargetResourceUtilization:         65,
		MaximumTargetResourceUtilization:         90,
		MinimumMinReplicas:                       3,
		PreferredMaxReplicas:                     30,
		MaximumCPUCores:                          "10",
		MinimumCPUCores:                          "50m",
		MinimumCPUCoresPerContainer:              map[string]string{},
		MaximumMemoryBytes:                       "10Gi",
		MinimumMemoryBytes:                       "50Mi",
		MinimumMemoryBytesPerContainer:           map[string]string{},
		TimeZone:                                 "Asia/Tokyo",
		TortoiseUpdateInterval:                   15 * time.Second,
		HPATargetUtilizationMaxIncrease:          5,
		HPATargetUtilizationUpdateInterval:       time.Hour * 24,
		MaximumMinReplicas:                       10,
		MaximumMaxReplicas:                       100,
		MaxAllowedScalingDownRatio:               0.8,
		IstioSidecarProxyDefaultCPU:              "100m",
		IstioSidecarProxyDefaultMemory:           "200Mi",
		MinimumCPULimitCores:                     "0",
		ResourceLimitMultiplier:                  map[string]int64{},
	}
}

// ParseConfig parses the config file (yaml) and returns Config.
func ParseConfig(path string) (*Config, error) {
	config := defaultConfig()
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

	if config.HPATargetUtilizationMaxIncrease > 100 || config.HPATargetUtilizationMaxIncrease <= 0 {
		return fmt.Errorf("HPATargetUtilizationMaxIncrease should be between 1 and 100")
	}

	// MinimumMinReplica < MaximumMinReplicas <= MaximumMaxReplicas
	if config.MinimumMinReplicas >= int(config.MaximumMinReplicas) {
		return fmt.Errorf("MinimumMinReplicas should be less than MaximumMinReplicas")
	}
	if config.MaximumMinReplicas > config.MaximumMaxReplicas {
		return fmt.Errorf("MaximumMinReplicas should be less than or equal to MaximumMaxReplicas")
	}
	if config.PreferredMaxReplicas >= int(config.MaximumMaxReplicas) {
		return fmt.Errorf("PreferredMaxReplicas should be less than MaximumMaxReplicas")
	}
	if config.PreferredMaxReplicas <= config.MinimumMinReplicas {
		return fmt.Errorf("PreferredMaxReplicas should be greater than or equal to MinimumMinReplicas")
	}

	if config.MaxAllowedScalingDownRatio < 0 || config.MaxAllowedScalingDownRatio > 1 {
		return fmt.Errorf("MaxAllowedScalingDownRatio should be between 0 and 1")
	}

	for _, ratio := range config.ResourceLimitMultiplier {
		if ratio < 1 {
			// ResourceLimitMultiplier should be greater than or equal to 1.
			// If it's less than 1, the resource limit will be less than the resource request, which doesn't make sense.
			return fmt.Errorf("ResourceLimitMultiplier should be greater than or equal to 1")
		}
	}

	return nil
}
