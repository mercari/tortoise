package deployment

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
)

type Service struct {
	c client.Client
}

func New(c client.Client) *Service {
	return &Service{c: c}
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
			names.Insert(annotation.IstioSidecarInjectionAnnotation)
		}
	}

	return names
}
