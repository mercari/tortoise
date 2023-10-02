package annotation

const (
	// TortoiseNameAnnotation - VPA and HPA managed by tortoise have this label.
	TortoiseNameAnnotation = "tortoises.autoscaling.mercari.com/tortoise-name"

	// If this annotation is set to "true", it means that tortoise manages that resource,
	// and will be removed when the tortoise is deleted.
	ManagedByTortoiseAnnotation = "tortoise.autoscaling.mercari.com/managed-by-tortoise"
)
