package annotation

const (
	// IstioSidecarInjectionAnnotation - If this annotation is set to "true", it means that the sidecar injection is enabled.
	IstioSidecarInjectionAnnotation = "sidecar.istio.io/inject"

	IstioSidecarProxyCPUAnnotation         = "sidecar.istio.io/proxyCPU"
	IstioSidecarProxyCPULimitAnnotation    = "sidecar.istio.io/proxyCPULimit"
	IstioSidecarProxyMemoryAnnotation      = "sidecar.istio.io/proxyMemory"
	IstioSidecarProxyMemoryLimitAnnotation = "sidecar.istio.io/proxyMemoryLimit"

	UpdatedAtAnnotation = "kubectl.kubernetes.io/restartedAt"
)
