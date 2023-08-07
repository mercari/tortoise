package hpa

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	"github.com/mercari/tortoise/metrics"
	"github.com/mercari/tortoise/pkg/annotation"
)

type Service struct {
	c client.Client

	replicaReductionFactor         float64
	upperTargetResourceUtilization int32
}

func New(c client.Client, replicaReductionFactor float64, upperTargetResourceUtilization int) *Service {
	return &Service{
		c:                              c,
		replicaReductionFactor:         replicaReductionFactor,
		upperTargetResourceUtilization: int32(upperTargetResourceUtilization),
	}
}

func (c *Service) CreateHPAForSingleContainer(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, dm *v1.Deployment) (*v2.HorizontalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	// TODO: make this default HPA spec configurable.
	hpa := &v2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName,
			Namespace: tortoise.Namespace,
			Annotations: map[string]string{
				annotation.TortoiseNameAnnotation: tortoise.Name,
			},
		},
		Spec: v2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.DeploymentName,
				APIVersion: "apps/v1",
			},
			MinReplicas: pointer.Int32(int32(math.Ceil(float64(dm.Status.Replicas) / 2.0))),
			MaxReplicas: dm.Status.Replicas * 2,
			Behavior: &v2.HorizontalPodAutoscalerBehavior{
				ScaleDown: &v2.HPAScalingRules{
					Policies: []v2.HPAScalingPolicy{
						{
							Type:          v2.PercentScalingPolicy,
							Value:         2,
							PeriodSeconds: 90,
						},
					},
				},
			},
		},
	}

	m := make([]v2.MetricSpec, 0, len(tortoise.Spec.ResourcePolicy))
	for _, policy := range tortoise.Spec.ResourcePolicy {
		for r, p := range policy.AutoscalingPolicy {
			value := pointer.Int32(50)
			if p != autoscalingv1alpha1.AutoscalingTypeHorizontal {
				value = pointer.Int32(c.upperTargetResourceUtilization)
			}
			m = append(m, v2.MetricSpec{
				Type: v2.ResourceMetricSourceType,
				Resource: &v2.ResourceMetricSource{
					Name: r,
					Target: v2.MetricTarget{
						Type:               v2.UtilizationMetricType,
						AverageUtilization: value,
					},
				},
			})
		}
	}
	hpa.Spec.Metrics = m
	tortoise.Status.Targets.HorizontalPodAutoscaler = hpa.Name

	err := c.c.Create(ctx, hpa)
	if apierrors.IsAlreadyExists(err) {
		// A user specified the existing HPA.
		return nil, tortoise, nil
	}

	return hpa.DeepCopy(), tortoise, err
}

func (c *Service) GiveAnnotationsOnHPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) error {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		return nil
	}

	updateFn := func() error {
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, client.ObjectKey{
			Namespace: tortoise.Namespace,
			Name:      *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName,
		}, hpa); err != nil {
			return fmt.Errorf("get hpa: %w", err)
		}
		hpa.Annotations[annotation.TortoiseNameAnnotation] = tortoise.Name
		return c.c.Update(ctx, hpa)
	}

	return retry.RetryOnConflict(retry.DefaultRetry, updateFn)
}

func (c *Service) CreateHPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, dm *v1.Deployment) (*v2.HorizontalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	if len(dm.Spec.Template.Spec.Containers) == 1 {
		return c.CreateHPAForSingleContainer(ctx, tortoise, dm)
	}
	return c.CreateHPAForMultipleContainer(ctx, tortoise, dm)
}

func (c *Service) CreateHPAForMultipleContainer(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, dm *v1.Deployment) (*v2.HorizontalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	// TODO: make this default HPA spec configurable.
	hpa := &v2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName,
			Namespace: tortoise.Namespace,
			Annotations: map[string]string{
				annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation: fmt.Sprintf("datadogmetric@%s:%s-memory-", tortoise.Namespace, tortoise.Spec.TargetRefs.DeploymentName),
				annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation:    fmt.Sprintf("datadogmetric@%s:%s-cpu-", tortoise.Namespace, tortoise.Spec.TargetRefs.DeploymentName),
				annotation.TortoiseNameAnnotation:                                    tortoise.Name,
			},
		},
		Spec: v2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.DeploymentName,
				APIVersion: "apps/v1",
			},
			MinReplicas: pointer.Int32(int32(math.Ceil(float64(dm.Status.Replicas) / 2.0))),
			MaxReplicas: dm.Status.Replicas * 2,
			Behavior: &v2.HorizontalPodAutoscalerBehavior{
				ScaleUp: &v2.HPAScalingRules{
					Policies: []v2.HPAScalingPolicy{
						{
							Type:          v2.PercentScalingPolicy,
							Value:         100,
							PeriodSeconds: 60,
						},
					},
				},
				ScaleDown: &v2.HPAScalingRules{
					Policies: []v2.HPAScalingPolicy{
						{
							Type:          v2.PercentScalingPolicy,
							Value:         2,
							PeriodSeconds: 90,
						},
					},
				},
			},
		},
	}

	m := make([]v2.MetricSpec, 0, len(tortoise.Spec.ResourcePolicy))
	for _, policy := range tortoise.Spec.ResourcePolicy {
		for r, p := range policy.AutoscalingPolicy {
			value := resourceQuantityPtr(resource.MustParse("50"))
			if p != autoscalingv1alpha1.AutoscalingTypeHorizontal {
				value = resourceQuantityPtr(resource.MustParse(strconv.Itoa(int(c.upperTargetResourceUtilization))))
			}
			externalMetricName, err := externalMetricNameFromAnnotation(hpa, policy.ContainerName, r)
			if err != nil {
				return nil, tortoise, err
			}
			m = append(m, v2.MetricSpec{
				Type: v2.ExternalMetricSourceType,
				External: &v2.ExternalMetricSource{
					Metric: v2.MetricIdentifier{
						Name: externalMetricName,
					},
					Target: v2.MetricTarget{
						Type:  v2.ValueMetricType,
						Value: value,
					},
				},
			})
		}
	}
	hpa.Spec.Metrics = m
	tortoise.Status.Targets.HorizontalPodAutoscaler = hpa.Name

	err := c.c.Create(ctx, hpa)
	if apierrors.IsAlreadyExists(err) {
		// A user specified the existing HPA.
		return nil, tortoise, nil
	}

	return hpa.DeepCopy(), tortoise, err
}

func (c *Service) GetHPAOnTortoise(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}
	return hpa, nil
}

func (c *Service) ChangeHPAFromTortoiseRecommendation(tortoise *autoscalingv1alpha1.Tortoise, hpa *v2.HorizontalPodAutoscaler, now time.Time, recordMetrics bool) (*v2.HorizontalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	if tortoise.Status.Recommendations.Horizontal == nil {
		return hpa, nil, nil
	}
	for _, t := range tortoise.Status.Recommendations.Horizontal.TargetUtilizations {
		for resourcename, targetutil := range t.TargetUtilization {
			metrics.AppliedHPATargetUtilization.WithLabelValues(tortoise.Name, tortoise.Namespace, t.ContainerName, resourcename.String(), hpa.Name).Observe(float64(targetutil))
			if err := updateHPATargetValue(hpa, t.ContainerName, resourcename, targetutil, len(tortoise.Spec.ResourcePolicy) == 1); err != nil {
				return nil, tortoise, fmt.Errorf("update HPA from the recommendation from tortoise")
			}
		}
	}

	max, err := GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MaxReplicas, now)
	if err != nil {
		return nil, tortoise, fmt.Errorf("get maxReplicas recommendation: %w", err)
	}
	hpa.Spec.MaxReplicas = max

	var min int32
	switch tortoise.Status.TortoisePhase {
	case autoscalingv1alpha1.TortoisePhaseEmergency:
		// when emergency mode, we set the same value on minReplicas.
		min = max
	case autoscalingv1alpha1.TortoisePhaseBackToNormal:
		idealMin, err := GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MinReplicas, now)
		if err != nil {
			return nil, tortoise, fmt.Errorf("get minReplicas recommendation: %w", err)
		}
		currentMin := *hpa.Spec.MinReplicas
		reduced := int32(math.Trunc(float64(currentMin) * c.replicaReductionFactor))
		if idealMin > reduced {
			min = idealMin
			// BackToNormal is finished
			tortoise.Status.TortoisePhase = autoscalingv1alpha1.TortoisePhaseWorking
		} else {
			min = reduced
		}
	default:
		min, err = GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MinReplicas, now)
		if err != nil {
			return nil, tortoise, fmt.Errorf("get minReplicas recommendation: %w", err)
		}
	}
	hpa.Spec.MinReplicas = &min
	metrics.AppliedHPAMinReplicass.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Observe(float64(*hpa.Spec.MinReplicas))
	metrics.AppliedHPAMaxReplicass.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Observe(float64(hpa.Spec.MaxReplicas))

	return hpa, tortoise, nil
}

func (c *Service) UpdateHPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1alpha1.Tortoise, error) {
	retTortoise := &autoscalingv1alpha1.Tortoise{}
	retHPA := &v2.HorizontalPodAutoscaler{}

	// we only want to record metric once in every reconcile loop.
	metricsRecorded := false
	updateFn := func() error {
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName}, hpa); err != nil {
			return fmt.Errorf("failed to get hpa on tortoise: %w", err)
		}

		hpa, tortoise, err := c.ChangeHPAFromTortoiseRecommendation(tortoise, hpa, now, !metricsRecorded)
		if err != nil {
			return fmt.Errorf("change HPA from tortoise recommendation: %w", err)
		}
		metricsRecorded = true
		retTortoise = tortoise
		retHPA = hpa

		return c.c.Update(ctx, hpa)
	}

	return retHPA, retTortoise, retry.RetryOnConflict(retry.DefaultRetry, updateFn)
}

// GetReplicasRecommendation finds the corresponding recommendations.
func GetReplicasRecommendation(recommendations []autoscalingv1alpha1.ReplicasRecommendation, now time.Time) (int32, error) {
	for _, r := range recommendations {
		if now.Hour() < r.To && now.Hour() >= r.From && now.Weekday().String() == r.WeekDay {
			return r.Value, nil
		}
	}
	return 0, errors.New("no recommendation slot")
}

func externalMetricNameFromAnnotation(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName) (string, error) {
	var prefix string
	switch k {
	case corev1.ResourceCPU:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation]
	case corev1.ResourceMemory:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation]
	default:
		return "", fmt.Errorf("non supported resource type: %s", k)
	}
	return prefix + containerName, nil
}

func updateHPATargetValue(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName, targetValue int32, isSingleContainerDeployment bool) error {
	for _, m := range hpa.Spec.Metrics {
		if isSingleContainerDeployment && m.Type == v2.ResourceMetricSourceType && m.Resource.Target.Type == v2.UtilizationMetricType && m.Resource.Name == k {
			m.Resource.Target.AverageUtilization = pointer.Int32(targetValue)
		}

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

	externalMetricName, err := externalMetricNameFromAnnotation(hpa, containerName, k)
	if err != nil {
		return err
	}

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

func resourceQuantityPtr(quantity resource.Quantity) *resource.Quantity {
	return &quantity
}
