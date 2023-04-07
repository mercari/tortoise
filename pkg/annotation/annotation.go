package annotation

const (
	HPAContainerBasedCPUExternalMetricNamePrefixAnnotation    = "tortoises.autoscaling.mercari.com/container-based-cpu-metric-prefix"
	HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation = "tortoises.autoscaling.mercari.com/container-based-memory-metric-prefix"

	// TortoiseNameAnnotation - VPA and HPA managed by tortoise have this label.
	TortoiseNameAnnotation = "tortoises.autoscaling.mercari.com/tortoise-name"
)
