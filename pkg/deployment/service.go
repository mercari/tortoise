package deployment

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/event"
)

type Service struct {
	c        client.Client
	recorder record.EventRecorder

	// IstioSidecarProxyDefaultCPU is the default CPU resource request of the istio sidecar proxy.
	istioSidecarProxyDefaultCPU string
	// IstioSidecarProxyDefaultMemory is the default Memory resource request of the istio sidecar proxy.
	istioSidecarProxyDefaultMemory string
}

func New(c client.Client, istioSidecarProxyDefaultCPU, istioSidecarProxyDefaultMemory string, recorder record.EventRecorder) *Service {
	return &Service{c: c, istioSidecarProxyDefaultCPU: istioSidecarProxyDefaultCPU, istioSidecarProxyDefaultMemory: istioSidecarProxyDefaultMemory, recorder: recorder}
}

func (c *Service) GetDeploymentOnTortoise(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.Deployment, error) {
	d := &v1.Deployment{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.ScaleTargetRef.Name}, d); err != nil {
		return nil, fmt.Errorf("failed to get deployment on tortoise: %w", err)
	}
	return d, nil
}

func (c *Service) RolloutRestart(ctx context.Context, dm *v1.Deployment, tortoise *autoscalingv1beta3.Tortoise) error {
	if dm.Spec.Template.ObjectMeta.Annotations == nil {
		dm.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	dm.Spec.Template.ObjectMeta.Annotations[annotation.UpdatedAtAnnotation] = time.Now().Format(time.RFC3339)

	if err := c.c.Update(ctx, dm); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, event.RestartDeployment, "Deployment is restarted to apply the recommendation from Tortoise")
	log.FromContext(ctx).Info("Deployment is restarted to apply the recommendation from Tortoise", "tortoise", tortoise)

	return nil
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
