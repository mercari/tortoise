package v1beta3

import (
	"context"
	"fmt"

	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mercari/tortoise/pkg/workload"
)

type service struct {
	c client.Client
}

func newService(c client.Client) *service {
	return &service{c: c}
}

// GetPodTemplateOnTortoise returns the Pod template of the scale target (Deployment or Rollout) of the tortoise.
func (c *service) GetPodTemplateOnTortoise(ctx context.Context, tortoise *Tortoise) (*corev1.PodTemplateSpec, error) {
	w, err := workload.Get(ctx, c.c, tortoise.Namespace, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind, tortoise.Spec.TargetRefs.ScaleTargetRef.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get scale target on tortoise: %w", err)
	}
	return w.PodTemplate(), nil
}

func (c *service) GetHPAFromUser(ctx context.Context, tortoise *Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		// user doesn't specify HPA.
		return nil, nil
	}

	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, client.ObjectKey{
		Namespace: tortoise.Namespace,
		Name:      *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName,
	}, hpa); err != nil {
		return nil, fmt.Errorf("get hpa: %w", err)
	}
	return hpa, nil
}
