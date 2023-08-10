package v1alpha1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type service struct {
	c client.Client
}

func newService(c client.Client) *service {
	return &service{c: c}
}

func (c *service) GetDeploymentOnTortoise(ctx context.Context, tortoise *Tortoise) (*v1.Deployment, error) {
	d := &v1.Deployment{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.DeploymentName}, d); err != nil {
		return nil, fmt.Errorf("failed to get deployment on tortoise: %w", err)
	}
	return d, nil
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
