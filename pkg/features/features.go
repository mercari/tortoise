package features

type FeatureFlag string

const (
	// Stage: alpha (default: disabled)
	// Description: Enable the feature to use the vertical scaling if the replica number reaches the preferred max replicas.
	// It aims to reduce the replica number by increasing the resource requests - preventing the situation that too many too small replicas are running.
	// Tracked at https://github.com/mercari/tortoise/issues/329.
	VerticalScalingBasedOnPreferredMaxReplicas FeatureFlag = "VerticalScalingBasedOnPreferredMaxReplicas"

	// Stage: alpha (default: disabled)
	// Description: Enable the feature to modify GOMAXPROCS/GOMEMLIMIT based on the resource requests in the Pod mutating webhook.
	// Tracked at:
	// - https://github.com/mercari/tortoise/issues/319
	// - https://github.com/mercari/tortoise/issues/320
	GolangEnvModification FeatureFlag = "GolangEnvModification"
)

func Contains(flags []FeatureFlag, flag FeatureFlag) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}
