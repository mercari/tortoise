package v1beta1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
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
	if tortoise.Spec.TargetRefs.ScaleTargetRef.Kind != "Deployment" {
		return nil, fmt.Errorf("target kind is not deployment: %s", tortoise.Spec.TargetRefs.ScaleTargetRef.Kind)
	}

	d := &v1.Deployment{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.ScaleTargetRef.Name}, d); err != nil {
		return nil, fmt.Errorf("failed to get deployment on tortoise: %w", err)
	}
	return d, nil
}
