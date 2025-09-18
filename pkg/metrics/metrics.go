package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ActualHPATargetUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actual_hpa_utilization_target",
		Help: "hpa utilization target values that hpa actually has",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name", "hpa_name"})

	ActualHPAMinReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actual_hpa_minreplicas",
		Help: "hpa minReplicas that hpa actually has",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ActualHPAMaxReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actual_hpa_maxreplicas",
		Help: "hpa maxReplicas that hpa actually has",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	AppliedHPATargetUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "applied_hpa_utilization_target",
		Help: "hpa utilization target values that tortoises actually applys to hpa",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name", "hpa_name"})

	AppliedHPAMinReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "applied_hpa_minreplicas",
		Help: "hpa minReplicas that tortoises actually applys to hpa",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	AppliedHPAMaxReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "applied_hpa_maxreplicas",
		Help: "hpa maxReplicas that tortoises actually applys to hpa",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	AppliedCPURequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "applied_cpu_request",
		Help: "cpu request (millicore) that tortoises actually applys",
	}, []string{"tortoise_name", "namespace", "container_name", "controller_name", "controller_kind"})

	AppliedMemoryRequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "applied_memory_request",
		Help: "memory request (byte) that tortoises actually applys",
	}, []string{"tortoise_name", "namespace", "container_name", "controller_name", "controller_kind"})

	DecreaseApplyCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "decrease_apply_counter",
		Help: "counter for number of resource decreases applied by tortoise",
	}, []string{"tortoise_name", "namespace"})

	IncreaseApplyCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "increase_apply_counter",
		Help: "counter for number of resource increases applied by tortoise",
	}, []string{"tortoise_name", "namespace"})

	NetHPAMinReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "net_hpa_minreplicas",
		Help: "net hpa minReplicas that tortoises actually applys to hpa",
	}, []string{"tortoise_name", "namespace", "hpa_name", "kube_deployment"})

	NetHPAMaxReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "net_hpa_maxreplicas",
		Help: "net hpa maxReplicas that tortoises actually applys to hpa",
	}, []string{"tortoise_name", "namespace", "hpa_name", "kube_deployment"})

	NetCPURequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "net_cpu_request",
		Help: "net cpu request (millicore) that tortoises actually applys",
	}, []string{"tortoise_name", "namespace", "container_name", "controller_name", "controller_kind"})

	NetMemoryRequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "net_memory_request",
		Help: "net memory request (byte) that tortoises actually applys",
	}, []string{"tortoise_name", "namespace", "container_name", "controller_name", "controller_kind"})

	ProposedHPATargetUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_hpa_utilization_target",
		Help: "recommended hpa utilization target values that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name", "resource_name", "hpa_name"})

	ProposedHPAMinReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_hpa_minreplicas",
		Help: "recommended hpa minReplicas that tortoises propose",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ProposedHPAMaxReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_hpa_maxreplicas",
		Help: "recommended hpa maxReplicas that tortoises propose",
	}, []string{"tortoise_name", "namespace", "hpa_name"})

	ProposedCPURequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_cpu_request",
		Help: "recommended cpu request (millicore) that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name", "controller_name", "controller_kind"})

	ProposedMemoryRequest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proposed_memory_request",
		Help: "recommended memory request (byte) that tortoises propose",
	}, []string{"tortoise_name", "namespace", "container_name", "controller_name", "controller_kind"})

	TortoiseNumber = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tortoise_number",
		Help: "the number of tortoise",
	}, []string{"tortoise_name", "namespace", "controller_name", "controller_kind", "update_mode", "tortoise_phase"})

	GlobalDisableMode = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "global_disable_mode",
		Help: "indicates if global disable mode is enabled (1=enabled, 0=disabled)",
	})
)

func init() {
	metrics.Registry.MustRegister(
		ActualHPAMaxReplicas,
		ActualHPAMinReplicas,
		ActualHPATargetUtilization,
		AppliedHPATargetUtilization,
		AppliedHPAMaxReplicas,
		AppliedHPAMinReplicas,
		AppliedCPURequest,
		AppliedMemoryRequest,
		IncreaseApplyCounter,
		DecreaseApplyCounter,
		NetHPAMaxReplicas,
		NetHPAMinReplicas,
		NetCPURequest,
		NetMemoryRequest,
		ProposedHPATargetUtilization,
		ProposedHPAMinReplicas,
		ProposedHPAMaxReplicas,
		ProposedCPURequest,
		ProposedMemoryRequest,
		TortoiseNumber,
		GlobalDisableMode,
	)
}

// SetGlobalDisableMode sets the global disable mode metric
func SetGlobalDisableMode(enabled bool) {
	if enabled {
		GlobalDisableMode.Set(1)
	} else {
		GlobalDisableMode.Set(0)
	}
}
