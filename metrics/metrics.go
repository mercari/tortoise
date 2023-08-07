package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	AppliedHPATargetUtilization = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "applied_hpa_utilization_target",
		Help: "recommended hpa utilization target values that tortoises apply",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name", "hpa_name"})

	AppliedHPAMinReplicass = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "applied_hpa_minreplicas",
		Help: "recommended hpa minReplicas that tortoises apply",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	AppliedHPAMaxReplicass = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "applied_hpa_maxreplicas",
		Help: "recommended hpa maxReplicas that tortoises apply",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	AppliedResourceRequest = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "applied_resource_request",
		Help: "recommended cpu request that tortoises apply",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name"})
)

func init() {
	//Register metrics with prometheus
	prometheus.MustRegister(
		AppliedHPATargetUtilization,
		AppliedHPAMinReplicass,
		AppliedHPAMaxReplicass,
		AppliedResourceRequest,
	)
}
