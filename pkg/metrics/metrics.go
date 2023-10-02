package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ProposedHPATargetUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_hpa_utilization_target",
		Help: "recommended hpa utilization target values that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name", "hpa_name"})

	ProposedHPAMinReplicass = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_hpa_minreplicas",
		Help: "recommended hpa minReplicas that tortoises propose",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ProposedHPAMaxReplicass = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_hpa_maxreplicas",
		Help: "recommended hpa maxReplicas that tortoises propose",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ProposedCPURequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_cpu_request",
		Help: "recommended resource request that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name"})

	ProposedMemoryRequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_memory_request",
		Help: "recommended resource request that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name"})
)

func init() {
	//Register metrics with prometheus
	metrics.Registry.MustRegister(
		ProposedHPATargetUtilization,
		ProposedHPAMinReplicass,
		ProposedHPAMaxReplicass,
		ProposedCPURequest,
		ProposedMemoryRequest,
	)
}
