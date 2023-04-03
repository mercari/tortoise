package hpa

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanposhiho/tortoise/pkg/annotation"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/types"

	autoscalingv1alpha1 "github.com/sanposhiho/tortoise/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	c client.Client
}

func New(c client.Client) *Client {
	return &Client{c: c}
}

const TortoiseHPANamePrefix = "tortoise-hpa-"

func TortoiseVPAName(tortoiseName string) string {
	return TortoiseHPANamePrefix + tortoiseName
}

func (c *Client) GetHPAOnTortoise(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}
	return hpa, nil
}

func (c *Client) UpdateHPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, now time.Time) (*v2.HorizontalPodAutoscaler, error) {
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}

	for _, t := range tortoise.Status.Recommendations.Horizontal.TargetUtilizations {
		for k, r := range t.TargetUtilization {
			if err := updateHPATargetValue(hpa, t.ContainerName, k, r); err != nil {
				return nil, fmt.Errorf("update HPA from the recommendation from tortoise")
			}
		}
	}

	max, err := getReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MaxReplicas, now)
	if err != nil {
		return nil, fmt.Errorf("get maxReplicas recommendation: %w", err)
	}
	hpa.Spec.MaxReplicas = max

	// when emergency mode, we set the same value on minReplicas.
	min := max
	if tortoise.Spec.UpdateMode != autoscalingv1alpha1.EmergencyMode {
		min, err = getReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MinReplicas, now)
		if err != nil {
			return nil, fmt.Errorf("get minReplicas recommendation: %w", err)
		}
	}
	hpa.Spec.MinReplicas = &min

	return hpa, c.c.Update(ctx, hpa)
}

// getReplicasRecommendation finds the corresponding recommendations.
func getReplicasRecommendation(recommendations []autoscalingv1alpha1.ReplicasRecommendation, now time.Time) (int32, error) {
	for _, r := range recommendations {
		if now.Hour() < r.To && now.Hour() >= r.From && now.Weekday() == r.WeekDay {
			return r.Value, nil
		}
	}
	return 0, errors.New("no recommendation slot")
}

func updateHPATargetValue(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName, targetValue int32) error {
	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ContainerResourceMetricSourceType {
			continue
		}

		if m.ContainerResource == nil {
			// shouldn't reach here
			klog.ErrorS(nil, "invalid container resource metric", klog.KObj(hpa))
			continue
		}

		if m.ContainerResource.Container != containerName || m.ContainerResource.Name != k || m.ContainerResource.Target.AverageUtilization == nil {
			continue
		}

		m.ContainerResource.Target.AverageUtilization = &targetValue
	}

	var prefix string
	switch k {
	case corev1.ResourceCPU:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation]
	case corev1.ResourceMemory:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation]
	default:
		return fmt.Errorf("non supported resource type: %s", k)
	}
	externalMetricName := prefix + containerName

	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ExternalMetricSourceType {
			continue
		}

		if m.External == nil {
			// shouldn't reach here
			klog.ErrorS(nil, "invalid external metric", klog.KObj(hpa))
			continue
		}

		if m.External.Metric.Name != externalMetricName {
			continue
		}

		m.External.Target.Value.Set(int64(targetValue))
	}

	return nil
}
