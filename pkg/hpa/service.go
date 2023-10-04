package hpa

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1beta1 "github.com/mercari/tortoise/api/v1beta1"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/metrics"
)

type Service struct {
	c client.Client

	replicaReductionFactor         float64
	upperTargetResourceUtilization int32
	recorder                       record.EventRecorder
}

func New(c client.Client, recorder record.EventRecorder, replicaReductionFactor float64, upperTargetResourceUtilization int) *Service {
	return &Service{
		c:                              c,
		replicaReductionFactor:         replicaReductionFactor,
		upperTargetResourceUtilization: int32(upperTargetResourceUtilization),
		recorder:                       recorder,
	}
}

func (c *Service) InitializeHPA(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise, dm *v1.Deployment) (*autoscalingv1beta1.Tortoise, error) {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		// update the existing HPA that the user set on tortoise.
		tortoise, err := c.giveAnnotationsOnExistingHPA(ctx, tortoise)
		if err != nil {
			return tortoise, fmt.Errorf("give annotations on a hpa specified in targetrefs: %w", err)
		}

		c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPAUpdated", fmt.Sprintf("Updated HPA %s/%s", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

		return tortoise, nil
	}

	// create default HPA.
	_, tortoise, err := c.CreateHPA(ctx, tortoise, dm)
	if err != nil {
		return tortoise, fmt.Errorf("create hpa: %w", err)
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPACreated", fmt.Sprintf("Initialized a HPA %s/%s", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

	return tortoise, nil
}

func (c *Service) giveAnnotationsOnExistingHPA(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise) (*autoscalingv1beta1.Tortoise, error) {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		// shouldn't reach here since the caller should check this.
		return tortoise, fmt.Errorf("tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName is nil")
	}
	updateFn := func() error {
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, client.ObjectKey{
			Namespace: tortoise.Namespace,
			Name:      *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName,
		}, hpa); err != nil {
			return fmt.Errorf("get hpa: %w", err)
		}
		if hpa.Annotations == nil {
			hpa.Annotations = map[string]string{}
		}
		hpa.Annotations[annotation.TortoiseNameAnnotation] = tortoise.Name
		hpa.Annotations[annotation.ManagedByTortoiseAnnotation] = "true"
		tortoise.Status.Targets.HorizontalPodAutoscaler = hpa.Name
		return c.c.Update(ctx, hpa)
	}

	return tortoise, retry.RetryOnConflict(retry.DefaultRetry, updateFn)
}

func (c *Service) DeleteHPACreatedByTortoise(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise) error {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil || tortoise.Spec.DeletionPolicy == autoscalingv1beta1.DeletionPolicyNoDelete {
		// A user specified the existing HPA and tortoise didn't create HPA by itself.
		return nil
	}

	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, client.ObjectKey{
		Namespace: tortoise.Namespace,
		Name:      tortoise.Status.Targets.HorizontalPodAutoscaler,
	}, hpa); err != nil {
		if apierrors.IsNotFound(err) {
			// already deleted
			return nil
		}
		return fmt.Errorf("failed to get hpa: %w", err)
	}

	// make sure it's created by tortoise
	if v, ok := hpa.Annotations[annotation.ManagedByTortoiseAnnotation]; !ok || v != "true" {
		// shouldn't reach here unless user manually remove the annotation.
		return nil
	}

	if err := c.c.Delete(ctx, hpa); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete hpa: %w", err)
	}

	return nil
}

func (c *Service) CreateHPA(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise, dm *v1.Deployment) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta1.Tortoise, error) {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		// we don't have to create HPA as the user specified the existing HPA.
		return nil, tortoise, nil
	}

	hpa := &v2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalingv1beta1.TortoiseDefaultHPAName(tortoise.Name),
			Namespace: tortoise.Namespace,
			Annotations: map[string]string{
				annotation.TortoiseNameAnnotation:      tortoise.Name,
				annotation.ManagedByTortoiseAnnotation: "true",
			},
		},
		Spec: v2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2.CrossVersionObjectReference{
				Kind:       tortoise.Spec.TargetRefs.ScaleTargetRef.Kind,
				Name:       tortoise.Spec.TargetRefs.ScaleTargetRef.Name,
				APIVersion: tortoise.Spec.TargetRefs.ScaleTargetRef.APIVersion,
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
			value := pointer.Int32(50)
			if p == autoscalingv1beta1.AutoscalingTypeVertical {
				value = pointer.Int32(c.upperTargetResourceUtilization)
			}
			m = append(m, v2.MetricSpec{
				Type: v2.ContainerResourceMetricSourceType,
				ContainerResource: &v2.ContainerResourceMetricSource{
					Name:      r,
					Container: policy.ContainerName,
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
	return hpa.DeepCopy(), tortoise, err
}

func (c *Service) GetHPAOnTortoise(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}
	return hpa, nil
}

func (c *Service) ChangeHPAFromTortoiseRecommendation(tortoise *autoscalingv1beta1.Tortoise, hpa *v2.HorizontalPodAutoscaler, now time.Time, recordMetrics bool) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta1.Tortoise, error) {
	for _, t := range tortoise.Status.Recommendations.Horizontal.TargetUtilizations {
		for resourcename, targetutil := range t.TargetUtilization {
			metrics.ProposedHPATargetUtilization.WithLabelValues(tortoise.Name, tortoise.Namespace, t.ContainerName, resourcename.String(), hpa.Name).Set(float64(targetutil))
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
	case autoscalingv1beta1.TortoisePhaseEmergency:
		// when emergency mode, we set the same value on minReplicas.
		min = max
	case autoscalingv1beta1.TortoisePhaseBackToNormal:
		idealMin, err := GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MinReplicas, now)
		if err != nil {
			return nil, tortoise, fmt.Errorf("get minReplicas recommendation: %w", err)
		}
		currentMin := *hpa.Spec.MinReplicas
		reduced := int32(math.Trunc(float64(currentMin) * c.replicaReductionFactor))
		if idealMin > reduced {
			min = idealMin
			// BackToNormal is finished
			tortoise.Status.TortoisePhase = autoscalingv1beta1.TortoisePhaseWorking
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
	metrics.ProposedHPAMinReplicass.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(*hpa.Spec.MinReplicas))
	metrics.ProposedHPAMaxReplicass.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(hpa.Spec.MaxReplicas))

	return hpa, tortoise, nil
}

func (c *Service) UpdateHPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta1.Tortoise, error) {
	// if all policy is off or Vertical, we don't update HPA.
	foundHorizontal := false
	for _, r := range tortoise.Spec.ResourcePolicy {
		for _, p := range r.AutoscalingPolicy {
			if p == autoscalingv1beta1.AutoscalingTypeHorizontal {
				foundHorizontal = true
				break
			}
		}
	}
	if !foundHorizontal {
		return nil, tortoise, nil
	}

	retTortoise := &autoscalingv1beta1.Tortoise{}
	retHPA := &v2.HorizontalPodAutoscaler{}

	// we only want to record metric once in every reconcile loop.
	metricsRecorded := false
	updateFn := func() error {
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
			return fmt.Errorf("failed to get hpa on tortoise: %w", err)
		}
		retHPA = hpa.DeepCopy()

		hpa, tortoise, err := c.ChangeHPAFromTortoiseRecommendation(tortoise, hpa, now, !metricsRecorded)
		if err != nil {
			return fmt.Errorf("change HPA from tortoise recommendation: %w", err)
		}
		metricsRecorded = true
		retTortoise = tortoise
		if tortoise.Spec.UpdateMode == autoscalingv1beta1.UpdateModeOff {
			// don't update status if update mode is off. (= dryrun)
			return nil
		}
		retHPA = hpa
		return c.c.Update(ctx, hpa)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return nil, nil, err
	}

	if tortoise.Spec.UpdateMode != autoscalingv1beta1.UpdateModeOff {
		c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPAUpdated", fmt.Sprintf("HPA %s/%s is updated by the recommendation", retHPA.Namespace, retHPA.Name))
	}

	return retHPA, retTortoise, nil
}

// GetReplicasRecommendation finds the corresponding recommendations.
func GetReplicasRecommendation(recommendations []autoscalingv1beta1.ReplicasRecommendation, now time.Time) (int32, error) {
	for _, r := range recommendations {
		if now.Hour() < r.To && now.Hour() >= r.From && (r.WeekDay == nil || now.Weekday().String() == *r.WeekDay) {
			return r.Value, nil
		}
	}
	return 0, errors.New("no recommendation slot")
}

func updateHPATargetValue(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName, targetValue int32, isSingleContainerDeployment bool) error {
	for _, m := range hpa.Spec.Metrics {
		if isSingleContainerDeployment && m.Type == v2.ResourceMetricSourceType && m.Resource.Target.Type == v2.UtilizationMetricType && m.Resource.Name == k {
			// If the deployment has only one container, the resource metric is the target.
			m.Resource.Target.AverageUtilization = pointer.Int32(targetValue)
		}
	}

	// If the deployment has more than one container, the container resource metric is the metric for the container.
	// Also, even if the deployment has only one container, the container resource metric might be used as well.
	// So, check the container resource metric as well.

	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ContainerResourceMetricSourceType {
			continue
		}

		if m.ContainerResource == nil {
			// shouldn't reach here
			return fmt.Errorf("invalid container resource metric: %s/%s", hpa.Namespace, hpa.Name)
		}

		if m.ContainerResource.Container != containerName || m.ContainerResource.Name != k || m.ContainerResource.Target.AverageUtilization == nil {
			continue
		}

		m.ContainerResource.Target.AverageUtilization = &targetValue
	}

	return nil
}
