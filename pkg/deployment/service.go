package deployment

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
)

type Service struct {
	c client.Client

	// IstioSidecarProxyDefaultCPU is the default CPU resource request of the istio sidecar proxy.
	istioSidecarProxyDefaultCPU string
	// IstioSidecarProxyDefaultMemory is the default Memory resource request of the istio sidecar proxy.
	istioSidecarProxyDefaultMemory string
}

func New(c client.Client, istioSidecarProxyDefaultCPU, istioSidecarProxyDefaultMemory string) *Service {
	return &Service{c: c, istioSidecarProxyDefaultCPU: istioSidecarProxyDefaultCPU, istioSidecarProxyDefaultMemory: istioSidecarProxyDefaultMemory}
}

func (c *Service) GetDeploymentOnTortoise(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.Deployment, error) {
	d := &v1.Deployment{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.ScaleTargetRef.Name}, d); err != nil {
		return nil, fmt.Errorf("failed to get deployment on tortoise: %w", err)
	}
	return d, nil
}

func GetContainerNames(dm *v1.Deployment) sets.Set[string] {
	names := sets.New[string]()
	for _, c := range dm.Spec.Template.Spec.Containers {
		names.Insert(c.Name)
	}
	if dm.Spec.Template.Annotations != nil {
		if v, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarInjectionAnnotation]; ok && v == "true" {
			// Istio sidecar injection is enabled.
			// Because the istio container spec is not in the deployment spec, we need to get it from the deployment's annotation.
			names.Insert("istio-proxy")
		}
	}

	return names
}

func (c *Service) GetResourceRequests(dm *v1.Deployment) ([]autoscalingv1beta3.ContainerResourceRequests, error) {
	actualContainerResource := []autoscalingv1beta3.ContainerResourceRequests{}
	for _, c := range dm.Spec.Template.Spec.Containers {
		rcr := autoscalingv1beta3.ContainerResourceRequests{
			ContainerName: c.Name,
			Resource:      corev1.ResourceList{},
		}
		for name, r := range c.Resources.Requests {
			rcr.Resource[name] = r
		}
		actualContainerResource = append(actualContainerResource, rcr)
	}

	if dm.Spec.Template.Annotations != nil {
		if v, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarInjectionAnnotation]; ok && v == "true" {
			// Istio sidecar injection is enabled.
			// Because the istio container spec is not in the deployment spec, we need to get it from the deployment's annotation.

			cpuReq, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarProxyCPUAnnotation]
			if !ok {
				cpuReq = c.istioSidecarProxyDefaultCPU
			}
			cpu, err := resource.ParseQuantity(cpuReq)
			if err != nil {
				return nil, fmt.Errorf("parse CPU request of istio sidecar: %w", err)
			}

			memoryReq, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarProxyMemoryAnnotation]
			if !ok {
				memoryReq = c.istioSidecarProxyDefaultMemory
			}
			memory, err := resource.ParseQuantity(memoryReq)
			if err != nil {
				return nil, fmt.Errorf("parse Memory request of istio sidecar: %w", err)
			}
			// If the deployment has the sidecar injection annotation, the Pods will have the sidecar container in addition.
			actualContainerResource = append(actualContainerResource, v1beta3.ContainerResourceRequests{
				ContainerName: "istio-proxy",
				Resource: corev1.ResourceList{
					corev1.ResourceCPU:    cpu,
					corev1.ResourceMemory: memory,
				},
			})
		}
	}
	return actualContainerResource, nil
}
