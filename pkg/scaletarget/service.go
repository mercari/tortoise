package scaletarget

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/event"
	"github.com/mercari/tortoise/pkg/workload"
)

// Service handles the scale target of Tortoise (Deployment or Argo Rollouts' Rollout).
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

// GetScaleTargetOnTortoise gets the workload defined in .spec.targetRefs.scaleTargetRef of the tortoise.
func (c *Service) GetScaleTargetOnTortoise(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*workload.Workload, error) {
	w, err := workload.Get(ctx, c.c, tortoise.Namespace, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind, tortoise.Spec.TargetRefs.ScaleTargetRef.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get scale target on tortoise: %w", err)
	}
	return w, nil
}

// UpdatePodTemplate writes the Pod template of the workload, which is modified via w.PodTemplate(), back to the cluster.
func (c *Service) UpdatePodTemplate(ctx context.Context, w *workload.Workload) error {
	return workload.UpdatePodTemplate(ctx, c.c, w)
}

// RolloutRestart restarts all the Pods of the workload so that they get the resource requests recommended by Tortoise.
func (c *Service) RolloutRestart(ctx context.Context, w *workload.Workload, tortoise *autoscalingv1beta3.Tortoise, now time.Time) error {
	if err := workload.Restart(ctx, c.c, w, now); err != nil {
		return err
	}

	reason := event.RestartDeployment
	if w.Kind() == workload.KindRollout {
		reason = event.RestartRollout
	}
	msg := fmt.Sprintf("%s is restarted to apply the recommendation from Tortoise", w.Kind())
	c.recorder.Event(tortoise, corev1.EventTypeNormal, reason, msg)
	log.FromContext(ctx).Info(msg, "tortoise", tortoise)

	return nil
}

// GetResourceRequests returns the resource requests of the containers in the workload.
func (c *Service) GetResourceRequests(w *workload.Workload) ([]autoscalingv1beta3.ContainerResourceRequests, error) {
	actualContainerResource := []autoscalingv1beta3.ContainerResourceRequests{}
	template := w.PodTemplate()

	istioProxyIndex := -1
	for i, c := range template.Spec.Containers {
		rcr := autoscalingv1beta3.ContainerResourceRequests{
			ContainerName: c.Name,
			Resource:      corev1.ResourceList{},
		}
		for name, r := range c.Resources.Requests {
			rcr.Resource[name] = r
		}
		actualContainerResource = append(actualContainerResource, rcr)
		if c.Name == "istio-proxy" {
			istioProxyIndex = i
		}
	}

	if template.Annotations != nil {
		if v, ok := template.Annotations[annotation.IstioSidecarInjectionAnnotation]; ok && v == "true" {
			// Istio sidecar injection is enabled.
			// Because the istio container spec is not in the workload spec, we need to get it from the Pod template's annotation.

			cpuReq, ok := template.Annotations[annotation.IstioSidecarProxyCPUAnnotation]
			if !ok {
				cpuReq = c.istioSidecarProxyDefaultCPU
			}
			cpu, err := resource.ParseQuantity(cpuReq)
			if err != nil {
				return nil, fmt.Errorf("parse CPU request of istio sidecar: %w", err)
			}

			memoryReq, ok := template.Annotations[annotation.IstioSidecarProxyMemoryAnnotation]
			if !ok {
				memoryReq = c.istioSidecarProxyDefaultMemory
			}
			memory, err := resource.ParseQuantity(memoryReq)
			if err != nil {
				return nil, fmt.Errorf("parse Memory request of istio sidecar: %w", err)
			}

			if istioProxyIndex == -1 {
				// If the workload has the sidecar injection annotation, the Pods will have the sidecar container in addition.
				actualContainerResource = append(actualContainerResource, autoscalingv1beta3.ContainerResourceRequests{
					ContainerName: "istio-proxy",
					Resource: corev1.ResourceList{
						corev1.ResourceCPU:    cpu,
						corev1.ResourceMemory: memory,
					},
				})
			} else {
				// the workload has the sidecar injection annotation and it's using the custom injection:
				// https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/#customizing-injection
				actualContainerResource[istioProxyIndex].Resource[corev1.ResourceCPU] = cpu
				actualContainerResource[istioProxyIndex].Resource[corev1.ResourceMemory] = memory
			}
		}
	}

	return actualContainerResource, nil
}
