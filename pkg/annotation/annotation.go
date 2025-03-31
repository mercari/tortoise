package annotation

// annotation on Pod, HPA and VPA resource.
const (
	PodMutationAnnotation = "tortoise.autoscaling.mercari.com/pod-mutation"
)

// annotation on Tortoise resource.
const (
	// If this annotation is set to "true", it means that dryrun tortoise will get changed in the autoscaling policy
	// when a corresponding HPA is changed.
	// e.g., If HPA is changed to have containerResource metric for "istio-proxy" cpu, tortoise will modify the autoscaling policy to have "Horizontal" to "istio-proxy" cpu.
	//
	// Background:
	// In mercari, we're using DryRun Tortoise to periodically check the recommendation from Tortoises.
	// Auto Tortoise is keep modifing HPAs to have the recommendation from Tortoise,
	// which means the autoscaling policy in Auto Tortoise is always consistent with metrics in HPAs.
	// Even if users manually add/remove metrics in HPAs, Auto Tortoise will revert the change soon.
	// But, DryRun Tortoise is not allowed to modify HPAs, and if users manually add/remove metrics in HPAs,
	// it could result in being inconsistent with the autoscaling policy in DryRun Tortoise.
	ModifyDryRunTortoiseWhenHPAIsChangedAnnotation = "tortoise.autoscaling.mercari.com/modify-dryrun-tortoise-when-hpa-is-changed"
)
