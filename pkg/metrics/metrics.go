package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ProposedHPATargetUtilization = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "proposed_hpa_utilization_target",
		Help: "recommended hpa utilization target values that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name", "hpa_name"})

	ProposedHPAMinReplicass = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "proposed_hpa_minreplicas",
		Help: "recommended hpa minReplicas that tortoises propose",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ProposedHPAMaxReplicass = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "proposed_hpa_maxreplicas",
		Help: "recommended hpa maxReplicas that tortoises propose",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ProposedResourceRequest = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "proposed_resource_request",
		Help: "recommended resource request that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name"})
)

func init() {
	//Register metrics with prometheus
	prometheus.MustRegister(
		ProposedHPATargetUtilization,
		ProposedHPAMinReplicass,
		ProposedHPAMaxReplicass,
		ProposedResourceRequest,
	)
}
