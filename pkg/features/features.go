package features

type FeatureFlag string

const (
	// Stage: alpha (default: disabled)
	// Description: Enable the feature to use the vertical scaling if the replica number reaches the preferred max replicas.
	// It aims to reduce the replica number by increasing the resource requests - preventing the situation that too many too small replicas are running.
	// Tracked at https://github.com/mercari/tortoise/issues/329.
	VerticalScalingBasedOnPreferredMaxReplicas FeatureFlag = "VerticalScalingBasedOnPreferredMaxReplicas"

	// Stage: alpha (default: disabled)
	// Description: Enable the feature to modify GOMEMLIMIT based on the memory request in the Pod mutating webhook.
	GoMemLimitModificationEnabled FeatureFlag = "GoMemLimitModificationEnabled"
)

func Contains(flags []FeatureFlag, flag FeatureFlag) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}
