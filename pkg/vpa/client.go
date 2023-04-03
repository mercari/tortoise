package vpa

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoscalingv1alpha1 "github.com/sanposhiho/tortoise/api/v1alpha1"
	autoscaling "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	c client.Client
}

func New(c client.Client) *Client {
	return &Client{c: c}
}

const TortoiseMonitorVPANamePrefix = "tortoise-monitor-"
const TortoiseUpdaterVPANamePrefix = "tortoise-updater-"
const TortoiseVPARecommenderName = "tortoise"

func TortoiseMonitorVPAName(tortoiseName string) string {
	return TortoiseMonitorVPANamePrefix + tortoiseName
}

func TortoiseUpdaterVPAName(tortoiseName string) string {
	return TortoiseUpdaterVPANamePrefix + tortoiseName
}

func (c *Client) CreateTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	auto := v1.UpdateModeAuto
	vpa := &v1.VerticalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tortoise.Namespace,
			Name:      TortoiseMonitorVPAName(tortoise.Name),
		},
		Spec: v1.VerticalPodAutoscalerSpec{
			Recommenders: []*v1.VerticalPodAutoscalerRecommenderSelector{
				{
					Name: TortoiseVPARecommenderName, // This VPA is managed by Tortoise.
				},
			},
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.DeploymentName,
				APIVersion: "apps/v1",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: &auto,
			},
			ResourcePolicy: &v1.PodResourcePolicy{},
		},
	}
	crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
	for _, c := range tortoise.Spec.ResourcePolicy {
		crp = append(crp, v1.ContainerResourcePolicy{
			ContainerName: c.ContainerName,
			MinAllowed:    c.MinAllocatedResources,
		})
	}
	vpa.Spec.ResourcePolicy.ContainerPolicies = crp

	return vpa, c.c.Create(ctx, vpa)
}

func (c *Client) CreateTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	off := v1.UpdateModeOff
	vpa := &v1.VerticalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tortoise.Namespace,
			Name:      TortoiseMonitorVPAName(tortoise.Name),
		},
		Spec: v1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.DeploymentName,
				APIVersion: "apps/v1",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: &off,
			},
			ResourcePolicy: &v1.PodResourcePolicy{},
		},
	}
	crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
	for _, c := range tortoise.Spec.ResourcePolicy {
		crp = append(crp, v1.ContainerResourcePolicy{
			ContainerName: c.ContainerName,
			MinAllowed:    c.MinAllocatedResources,
		})
	}
	vpa.Spec.ResourcePolicy.ContainerPolicies = crp

	return vpa, c.c.Create(ctx, vpa)
}

func (c *Client) UpdateVPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	vpa, err := c.GetTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		return nil, fmt.Errorf("get tortoise VPA: %w", err)
	}
	newRecommendations := make([]v1.RecommendedContainerResources, 0, len(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation))
	for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
		newRecommendations = append(newRecommendations, v1.RecommendedContainerResources{
			ContainerName:  r.ContainerName,
			Target:         r.RecommendedResource,
			LowerBound:     r.RecommendedResource,
			UpperBound:     r.RecommendedResource,
			UncappedTarget: r.RecommendedResource,
		})
	}
	vpa.Status.Recommendation.ContainerRecommendations = newRecommendations

	return vpa, c.c.Status().Update(ctx, vpa)
}

func (c *Client) GetTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	vpa := &v1.VerticalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: TortoiseUpdaterVPAName(tortoise.Name)}, vpa); err != nil {
		return nil, fmt.Errorf("failed to get updater vpa on tortoise: %w", err)
	}
	return vpa, nil
}

func (c *Client) GetTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	vpa := &v1.VerticalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: TortoiseMonitorVPAName(tortoise.Name)}, vpa); err != nil {
		return nil, fmt.Errorf("failed to get vpa managed by tortoise controller: %w", err)
	}
	return vpa, nil
}
