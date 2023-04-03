package deployment

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/types"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	c client.Client
}

func New(c client.Client) *Client {
	return &Client{c: c}
}

func (c *Client) GetDeploymentOnTortoise(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.Deployment, error) {
	d := &v1.Deployment{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.DeploymentName}, d); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}
	return d, nil
}
