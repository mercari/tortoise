package deployment

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/types"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	c client.Client
}

func New(c client.Client) *Service {
	return &Service{c: c}
}

func (c *Service) GetDeploymentOnTortoise(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.Deployment, error) {
	d := &v1.Deployment{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.DeploymentName}, d); err != nil {
		return nil, fmt.Errorf("failed to get deployment on tortoise: %w", err)
	}
	return d, nil
}
