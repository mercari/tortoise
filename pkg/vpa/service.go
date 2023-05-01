package vpa

import (
	"context"
	"fmt"
	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	autoscaling "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

type Service struct {
	c versioned.Interface
}

func New(c *rest.Config) (*Service, error) {
	cli, err := versioned.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	return &Service{c: cli}, nil
}

const TortoiseMonitorVPANamePrefix = "tortoise-monitor-"
const TortoiseUpdaterVPANamePrefix = "tortoise-updater-"
const TortoiseVPARecommenderName = "tortoise-controller"

func TortoiseMonitorVPAName(tortoiseName string) string {
	return TortoiseMonitorVPANamePrefix + tortoiseName
}

func TortoiseUpdaterVPAName(tortoiseName string) string {
	return TortoiseUpdaterVPANamePrefix + tortoiseName
}

func (c *Service) CreateTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	auto := v1.UpdateModeAuto
	vpa := &v1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tortoise.Namespace,
			Name:      TortoiseUpdaterVPAName(tortoise.Name),
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

	tortoise.Status.Targets.VerticalPodAutoscalers = append(tortoise.Status.Targets.VerticalPodAutoscalers, autoscalingv1alpha1.TargetStatusVerticalPodAutoscaler{
		Name: vpa.Name,
		Role: autoscalingv1alpha1.VerticalPodAutoscalerRoleUpdater,
	})
	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Create(ctx, vpa, metav1.CreateOptions{})
	return vpa, tortoise, err
}

func (c *Service) CreateTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	off := v1.UpdateModeOff
	vpa := &v1.VerticalPodAutoscaler{
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

	tortoise.Status.Targets.VerticalPodAutoscalers = append(tortoise.Status.Targets.VerticalPodAutoscalers, autoscalingv1alpha1.TargetStatusVerticalPodAutoscaler{
		Name: vpa.Name,
		Role: autoscalingv1alpha1.VerticalPodAutoscalerRoleMonitor,
	})

	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Create(ctx, vpa, metav1.CreateOptions{})
	return vpa, tortoise, err
}

func (c *Service) UpdateVPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	retVPA := &v1.VerticalPodAutoscaler{}

	updateFn := func() error {
		vpa, err := c.GetTortoiseUpdaterVPA(ctx, tortoise)
		if err != nil {
			return fmt.Errorf("get tortoise VPA: %w", err)
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
		if vpa.Status.Recommendation == nil {
			vpa.Status.Recommendation = &v1.RecommendedPodResources{}
		}
		vpa.Status.Recommendation.ContainerRecommendations = newRecommendations
		retVPA, err = c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).UpdateStatus(ctx, vpa, metav1.UpdateOptions{})
		return err
	}

	return retVPA, retry.RetryOnConflict(retry.DefaultRetry, updateFn)
}

func (c *Service) GetTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Get(ctx, TortoiseUpdaterVPAName(tortoise.Name), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get updater vpa on tortoise: %w", err)
	}
	return vpa, nil
}

func (c *Service) GetTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v1.VerticalPodAutoscaler, bool, error) {
	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Get(ctx, TortoiseMonitorVPAName(tortoise.Name), metav1.GetOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("failed to get updater vpa on tortoise: %w", err)
	}

	for _, c := range vpa.Status.Conditions {
		if c.Type == v1.RecommendationProvided && c.Status == corev1.ConditionTrue {
			return vpa, true, nil
		}
	}

	return vpa, false, nil
}
