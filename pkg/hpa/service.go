package hpa

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"

	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/event"
	"github.com/mercari/tortoise/pkg/metrics"
	"github.com/mercari/tortoise/pkg/utils"
)

type Service struct {
	c client.Client

	replicaReductionFactor                     float64
	maximumTargetResourceUtilization           int32
	tortoiseHPATargetUtilizationMaxIncrease    int
	recorder                                   record.EventRecorder
	tortoiseHPATargetUtilizationUpdateInterval time.Duration
	minimumMinReplicas                         int32
	maximumMinReplica                          int32
	maximumMaxReplica                          int32
	externalMetricExclusionRegex               *regexp.Regexp
}

func New(
	c client.Client,
	recorder record.EventRecorder,
	replicaReductionFactor float64,
	maximumTargetResourceUtilization,
	tortoiseHPATargetUtilizationMaxIncrease int,
	tortoiseHPATargetUtilizationUpdateInterval time.Duration,
	maximumMinReplica, maximumMaxReplica int32,
	minimumMinReplicas int32,
	externalMetricExclusionRegex string,
) (*Service, error) {
	var regex *regexp.Regexp
	if externalMetricExclusionRegex != "" {
		var err error
		// parse the regex
		regex, err = regexp.Compile(externalMetricExclusionRegex)
		if err != nil {
			return nil, fmt.Errorf("failed to compile regex: %w", err)
		}
	}

	return &Service{
		c:                                       c,
		replicaReductionFactor:                  replicaReductionFactor,
		maximumTargetResourceUtilization:        int32(maximumTargetResourceUtilization),
		tortoiseHPATargetUtilizationMaxIncrease: tortoiseHPATargetUtilizationMaxIncrease,
		recorder:                                recorder,
		tortoiseHPATargetUtilizationUpdateInterval: tortoiseHPATargetUtilizationUpdateInterval,
		maximumMinReplica:                          maximumMinReplica,
		minimumMinReplicas:                         minimumMinReplicas,
		maximumMaxReplica:                          maximumMaxReplica,
		externalMetricExclusionRegex:               regex,
	}, nil
}

func (c *Service) InitializeHPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, replicaNum int32, now time.Time) (*autoscalingv1beta3.Tortoise, error) {
	logger := log.FromContext(ctx)
	// if all policy is off or Vertical, we don't need HPA.
	if !HasHorizontal(tortoise) {
		logger.Info("no horizontal policy, no need to create HPA")
		return tortoise, nil
	}

	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		logger.Info("user specified the existing HPA, no need to create HPA")

		tortoise.Status.Targets.HorizontalPodAutoscaler = *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName

		return tortoise, nil
	}

	logger.Info("no existing HPA specified, creating HPA")

	// create default HPA.
	_, tortoise, err := c.CreateHPA(ctx, tortoise, replicaNum, now)
	if err != nil {
		return tortoise, fmt.Errorf("create hpa: %w", err)
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, event.HPACreated, fmt.Sprintf("Initialized a HPA %s/%s for a created tortoise", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

	return tortoise, nil
}

func (c *Service) DeleteHPACreatedByTortoise(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) error {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		// The user specified the existing HPA, so we shouldn't delete it.
		return nil
	}
	if tortoise.Spec.DeletionPolicy == autoscalingv1beta3.DeletionPolicyNoDelete {
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

	if err := c.c.Delete(ctx, hpa); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete hpa: %w", err)
	}

	return nil
}

type resourceNameAndContainerName struct {
	rn            corev1.ResourceName
	containerName string
}

// syncHPAMetricsWithTortoiseAutoscalingPolicy adds metrics to the HPA based on the autoscaling policy in the tortoise.
// Note that it doesn't update the HPA in kube-apiserver, you have to do that after this function.
func (c *Service) syncHPAMetricsWithTortoiseAutoscalingPolicy(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, currenthpa *v2.HorizontalPodAutoscaler, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta3.Tortoise, bool) {
	currenthpa = currenthpa.DeepCopy()
	hpaEdited := false

	policies := sets.New[string]()
	horizontalResourceAndContainer := sets.New[resourceNameAndContainerName]()
	for _, p := range tortoise.Status.AutoscalingPolicy {
		policies.Insert(p.ContainerName)
		for rn, ap := range p.Policy {
			if ap == autoscalingv1beta3.AutoscalingTypeHorizontal {
				horizontalResourceAndContainer.Insert(resourceNameAndContainerName{rn, p.ContainerName})
			}
		}
	}

	hpaManagedResourceAndContainer := sets.New[resourceNameAndContainerName]()
	for _, m := range currenthpa.Spec.Metrics {
		if m.Type != v2.ContainerResourceMetricSourceType {
			continue
		}
		hpaManagedResourceAndContainer.Insert(resourceNameAndContainerName{m.ContainerResource.Name, m.ContainerResource.Container})
	}

	needToAddToHPA := horizontalResourceAndContainer.Difference(hpaManagedResourceAndContainer)
	needToRemoveFromHPA := hpaManagedResourceAndContainer.Difference(horizontalResourceAndContainer)

	sortedNeedToAddToHPA := needToAddToHPA.UnsortedList()
	sort.SliceStable(sortedNeedToAddToHPA, func(i, j int) bool {
		return sortedNeedToAddToHPA[i].containerName < sortedNeedToAddToHPA[j].containerName
	})

	// add metrics
	for _, d := range sortedNeedToAddToHPA {
		m := v2.MetricSpec{
			Type: v2.ContainerResourceMetricSourceType,
			ContainerResource: &v2.ContainerResourceMetricSource{
				Name:      d.rn,
				Container: d.containerName,
				Target: v2.MetricTarget{
					Type: v2.UtilizationMetricType,
					// we always start from a conservative value. and later will be adjusted by the recommendation.
					AverageUtilization: ptr.To[int32](70),
				},
			},
		}
		currenthpa.Spec.Metrics = append(currenthpa.Spec.Metrics, m)
		hpaEdited = true
		tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, d.containerName, d.rn, now, v1beta3.ContainerResourcePhaseGatheringData)
	}

	// remove metrics
	newMetrics := []v2.MetricSpec{}
	for _, m := range currenthpa.Spec.Metrics {
		if m.Type == v2.ResourceMetricSourceType {
			// resource metrics should be removed.
			continue
		}
		if m.Type != v2.ContainerResourceMetricSourceType {
			// We keep container resource metrics.
			newMetrics = append(newMetrics, m)
			continue
		}

		if !needToRemoveFromHPA.Has(resourceNameAndContainerName{m.ContainerResource.Name, m.ContainerResource.Container}) {
			newMetrics = append(newMetrics, m)
			hpaEdited = true
			continue
		}
	}
	currenthpa.Spec.Metrics = newMetrics

	return currenthpa, tortoise, hpaEdited
}

// TODO: move this to configuration
var globalRecommendedHPABehavior = &v2.HorizontalPodAutoscalerBehavior{
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
}

func (c *Service) CreateHPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, replicaNum int32, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta3.Tortoise, error) {
	if !HasHorizontal(tortoise) {
		// no need to create HPA
		return nil, tortoise, nil
	}
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		// we don't have to create HPA as the user specified the existing HPA.
		return nil, tortoise, nil
	}

	hpa := &v2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalingv1beta3.TortoiseDefaultHPAName(tortoise.Name),
			Namespace: tortoise.Namespace,
		},
		Spec: v2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2.CrossVersionObjectReference{
				Kind:       tortoise.Spec.TargetRefs.ScaleTargetRef.Kind,
				Name:       tortoise.Spec.TargetRefs.ScaleTargetRef.Name,
				APIVersion: tortoise.Spec.TargetRefs.ScaleTargetRef.APIVersion,
			},
			MinReplicas: ptr.To[int32](c.minimumMinReplicas),
			MaxReplicas: c.maximumMaxReplica,
			Behavior:    globalRecommendedHPABehavior,
		},
	}

	hpa, tortoise, _ = c.syncHPAMetricsWithTortoiseAutoscalingPolicy(ctx, tortoise, hpa, now)

	tortoise.Status.Targets.HorizontalPodAutoscaler = hpa.Name

	err := c.c.Create(ctx, hpa)
	return hpa.DeepCopy(), tortoise, err
}

func (c *Service) GetHPAOnTortoiseSpec(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		return nil, nil
	}
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}
	return hpa, nil
}

func (c *Service) GetHPAOnTortoise(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	if !HasHorizontal(tortoise) {
		// there should be no HPA
		return nil, nil
	}
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}

	recordHPAMetric(ctx, tortoise, hpa)

	return hpa, nil
}

func (s *Service) UpdatingHPATargetUtilizationAllowed(tortoise *autoscalingv1beta3.Tortoise, now time.Time) (*autoscalingv1beta3.Tortoise, bool) {
	for i, c := range tortoise.Status.Conditions.TortoiseConditions {
		if c.Type == autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated {
			if c.Type == autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated {
				tortoise.Status.Conditions.TortoiseConditions[i].LastUpdateTime = metav1.NewTime(now) // We always update the LastUpdateTime, regardless of whether the update would be allowed or not.
				// if the last LastTransitionTime is within the interval, we don't update the HPA.
				return tortoise, c.LastTransitionTime.Add(s.tortoiseHPATargetUtilizationUpdateInterval).Before(now) // And, we use LastTransitionTime to decide whether we should update the HPA or not.
			}
		}
	}
	// It's the first time to update the HPA. (Or someone modified the status)
	return tortoise, true
}

func (s *Service) RecordHPATargetUtilizationUpdate(tortoise *autoscalingv1beta3.Tortoise, now time.Time) *autoscalingv1beta3.Tortoise {
	for i, c := range tortoise.Status.Conditions.TortoiseConditions {
		if c.Type == autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated {
			tortoise.Status.Conditions.TortoiseConditions[i].LastTransitionTime = metav1.NewTime(now)
			tortoise.Status.Conditions.TortoiseConditions[i].LastUpdateTime = metav1.NewTime(now)
			return tortoise
		}
	}

	// It's the first time to update the HPA. (Or someone modified the status)
	tortoise.Status.Conditions.TortoiseConditions = append(tortoise.Status.Conditions.TortoiseConditions, autoscalingv1beta3.TortoiseCondition{
		Type:               autoscalingv1beta3.TortoiseConditionTypeHPATargetUtilizationUpdated,
		Status:             corev1.ConditionTrue,
		LastUpdateTime:     metav1.NewTime(now),
		LastTransitionTime: metav1.NewTime(now),
		Reason:             "HPATargetUtilizationUpdated",
		Message:            "HPA target utilization is updated",
	})
	return tortoise
}

func (c *Service) ChangeHPAFromTortoiseRecommendation(tortoise *autoscalingv1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, now time.Time, recordMetrics bool) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta3.Tortoise, error) {
	if tortoise.Status.TortoisePhase == v1beta3.TortoisePhaseInitializing || tortoise.Status.TortoisePhase == "" || tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
		// Tortoise is not ready, don't update HPA
		return hpa, tortoise, nil
	}

	readyHorizontalResourceAndContainer := sets.New[resourceNameAndContainerName]()
	for _, p := range tortoise.Status.AutoscalingPolicy {
		for rn, ap := range p.Policy {
			if ap == autoscalingv1beta3.AutoscalingTypeHorizontal {
				readyHorizontalResourceAndContainer.Insert(resourceNameAndContainerName{rn, p.ContainerName})
			}
		}
	}
	for _, p := range tortoise.Status.ContainerResourcePhases {
		for rn, phase := range p.ResourcePhases {
			if phase.Phase != autoscalingv1beta3.ContainerResourcePhaseWorking {
				readyHorizontalResourceAndContainer.Delete(resourceNameAndContainerName{rn, p.ContainerName})
			}
		}
	}
	if readyHorizontalResourceAndContainer.Len() == 0 {
		// all horizontal are not ready, don't update HPA
		return hpa, tortoise, nil
	}

	var allowed bool
	tortoise, allowed = c.UpdatingHPATargetUtilizationAllowed(tortoise, now)
	for _, t := range tortoise.Status.Recommendations.Horizontal.TargetUtilizations {
		for resourcename, proposedTarget := range t.TargetUtilization {
			if !readyHorizontalResourceAndContainer.Has(resourceNameAndContainerName{resourcename, t.ContainerName}) {
				// this recommendation is not ready. We don't want to apply it.
				continue
			}

			metrics.ProposedHPATargetUtilization.WithLabelValues(tortoise.Name, tortoise.Namespace, t.ContainerName, resourcename.String(), hpa.Name).Set(float64(proposedTarget))
			if !allowed {
				// we don't want to update the HPA too frequently.
				// But, we record the proposed HPA target utilization in metrics.
				continue
			}

			if allowed && tortoise.Spec.UpdateMode != autoscalingv1beta3.UpdateModeOff {
				metrics.AppliedHPATargetUtilization.WithLabelValues(tortoise.Name, tortoise.Namespace, t.ContainerName, resourcename.String(), hpa.Name).Set(float64(proposedTarget))
			}

			if err := c.updateHPATargetValue(hpa, t.ContainerName, resourcename, proposedTarget); err != nil {
				return nil, tortoise, fmt.Errorf("update HPA from the recommendation from tortoise")
			}

		}
	}
	if allowed && tortoise.Spec.UpdateMode != autoscalingv1beta3.UpdateModeOff {
		tortoise = c.RecordHPATargetUtilizationUpdate(tortoise, now)
	}

	recommendMax, err := GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MaxReplicas, now)
	if err != nil {
		return nil, tortoise, fmt.Errorf("get maxReplicas recommendation: %w", err)
	}

	if recommendMax > c.maximumMaxReplica {
		c.recorder.Event(tortoise, corev1.EventTypeWarning, event.WarningHittingHardMaxReplicaLimit, fmt.Sprintf("MaxReplica (%v) suggested from Tortoise (%s/%s) hits a cluster-wide maximum replica number (%v). It wouldn't be a problem until the replica number actually grows to %v though, you may want to reach out to your cluster admin.", recommendMax, tortoise.Namespace, tortoise.Name, c.maximumMaxReplica, c.maximumMaxReplica))
		recommendMax = c.maximumMaxReplica
	}

	hpa.Spec.MaxReplicas = recommendMax

	recommendMin, err := GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MinReplicas, now)
	if err != nil {
		return nil, tortoise, fmt.Errorf("get minReplicas recommendation: %w", err)
	}
	if recommendMin > c.maximumMinReplica {
		recommendMin = c.maximumMinReplica
		// We don't change the maxReplica because it's dangerous to limit.
	}

	if recordMetrics {
		metrics.ProposedHPAMinReplicas.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(recommendMin))
		metrics.ProposedHPAMaxReplicas.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(recommendMax))
	}

	// the minReplicas to be applied is not always the same as the recommended one.
	var minToActuallyApply int32
	switch tortoise.Status.TortoisePhase {
	case autoscalingv1beta3.TortoisePhaseEmergency:
		// when emergency mode, we set the same value on minReplicas.
		minToActuallyApply = recommendMax
	case autoscalingv1beta3.TortoisePhaseBackToNormal:
		// gradually reduce the minReplicas.
		currentMin := *hpa.Spec.MinReplicas
		reduced := int32(math.Trunc(float64(currentMin) * c.replicaReductionFactor))
		if recommendMin > reduced {
			minToActuallyApply = recommendMin
			// BackToNormal is finished
			tortoise.Status.TortoisePhase = autoscalingv1beta3.TortoisePhaseWorking
			c.recorder.Event(tortoise, corev1.EventTypeNormal, event.Working, fmt.Sprintf("Tortoise %s/%s is working %v", tortoise.Namespace, tortoise.Name, currentMin))
		} else {
			minToActuallyApply = reduced
		}
	default:
		minToActuallyApply = recommendMin
	}

	hpa.Spec.MinReplicas = &minToActuallyApply
	if tortoise.Spec.UpdateMode != autoscalingv1beta3.UpdateModeOff && recordMetrics {
		// We don't want to record applied* metric when UpdateMode is Off.
		metrics.AppliedHPAMinReplicas.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(*hpa.Spec.MinReplicas))
		metrics.AppliedHPAMaxReplicas.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(hpa.Spec.MaxReplicas))
	}

	return hpa, tortoise, nil
}

// disableHPA disables the HPA created by users without removing it, by removing all metrics and setting the minReplicas to the specified value.
func (c *Service) disableHPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, replicaNum int32) error {
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		// nothing to do.
		return nil
	}
	updateFn := func() error {
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName}, hpa); err != nil {
			return fmt.Errorf("failed to get hpa on tortoise: %w", err)
		}

		hpa.Spec.Metrics = nil
		hpa.Spec.MaxReplicas = replicaNum
		hpa.Spec.MinReplicas = ptr.To(replicaNum)

		return c.c.Update(ctx, hpa)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return err
	}

	return nil
}

func (c *Service) UpdateHPASpecFromTortoiseAutoscalingPolicy(
	ctx context.Context,
	tortoise *autoscalingv1beta3.Tortoise,
	// givenHPA is the HPA that is specified in the targetRefs.
	// If the user didn't specify the HPA, it's nil.
	givenHPA *v2.HorizontalPodAutoscaler,
	replicaNum int32,
	now time.Time,
) (*autoscalingv1beta3.Tortoise, error) {
	if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
		// When UpdateMode is Off, we don't update HPA.
		return tortoise, nil
	}

	if !HasHorizontal(tortoise) {
		if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
			// HPA should be created by Tortoise, which can be deleted.
			err := c.DeleteHPACreatedByTortoise(ctx, tortoise)
			if err != nil && !apierrors.IsNotFound(err) {
				return tortoise, fmt.Errorf("delete hpa created by tortoise: %w", err)
			}
			c.recorder.Event(tortoise, corev1.EventTypeNormal, event.HPADeleted, fmt.Sprintf("Deleted a HPA %s/%s because tortoise has no resource to scale horizontally", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))
		} else {
			// We cannot delete the HPA because it's specified by the user.
			err := c.disableHPA(ctx, tortoise, replicaNum)
			if err != nil {
				return tortoise, fmt.Errorf("disable hpa: %w", err)
			}
			c.recorder.Event(tortoise, corev1.EventTypeNormal, event.HPADisabled, fmt.Sprintf("Disabled a HPA %s/%s because tortoise has no resource to scale horizontally", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))
		}

		// No need to edit container resource phase.

		return tortoise, nil
	}

	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
		if apierrors.IsNotFound(err) {
			// If not found, it's one of:
			// - the user didn't specify Horizontal in any autoscalingPolicy previously,
			//   but just updated tortoise to have Horizontal in some.
			//   - In that case, we need to create an initial HPA or give an annotation to existing HPA.
			tortoise, err = c.InitializeHPA(ctx, tortoise, replicaNum, now)
			if err != nil {
				return tortoise, fmt.Errorf("initialize hpa: %w", err)
			}

			c.recorder.Event(tortoise, corev1.EventTypeNormal, event.HPACreated, fmt.Sprintf("Initialized a HPA %s/%s because tortoise has resource to scale horizontally", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))
			return tortoise, nil
		}

		return tortoise, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}

	var newhpa *v2.HorizontalPodAutoscaler
	var isHpaEdited bool
	newhpa, tortoise, isHpaEdited = c.syncHPAMetricsWithTortoiseAutoscalingPolicy(ctx, tortoise, hpa, now)
	if !isHpaEdited {
		// User didn't change anything.
		return tortoise, nil
	}

	retryNumber := -1
	updateFn := func() error {
		retryNumber++
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
			return fmt.Errorf("failed to get hpa on tortoise: %w", err)
		}

		hpa = hpa.DeepCopy()
		// update only metrics
		hpa.Spec.Metrics = newhpa.Spec.Metrics

		return c.c.Update(ctx, hpa)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return tortoise, fmt.Errorf("update hpa: %w (%v times retried)", err, replicaNum)
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, event.HPAUpdated, fmt.Sprintf("Updated a HPA %s/%s because the autoscaling policy is changed in the tortoise", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

	return tortoise, nil
}

func HasHorizontal(tortoise *autoscalingv1beta3.Tortoise) bool {
	for _, r := range tortoise.Status.AutoscalingPolicy {
		for _, p := range r.Policy {
			if p == autoscalingv1beta3.AutoscalingTypeHorizontal {
				return true
			}
		}
	}
	return false
}

func (c *Service) UpdateHPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta3.Tortoise, error) {
	// if all policy is off or Vertical, we don't update HPA.
	if !HasHorizontal(tortoise) {
		return nil, tortoise, nil
	}

	retTortoise := &autoscalingv1beta3.Tortoise{}
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
		hpa.Spec.Behavior = globalRecommendedHPABehavior // overwrite
		retTortoise = tortoise
		if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
			// don't update status if update mode is off. (= dryrun)
			return nil
		}

		hpa = c.excludeExternalMetric(ctx, hpa)
		retHPA = hpa
		return c.c.Update(ctx, hpa)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return nil, retTortoise, err
	}

	if tortoise.Spec.UpdateMode != autoscalingv1beta3.UpdateModeOff {
		c.recorder.Event(tortoise, corev1.EventTypeNormal, event.HPAUpdated, fmt.Sprintf("HPA %s/%s is updated by the recommendation", retHPA.Namespace, retHPA.Name))
	}

	return retHPA, retTortoise, nil
}

// GetReplicasRecommendation finds the corresponding recommendations.
func GetReplicasRecommendation(recommendations []autoscalingv1beta3.ReplicasRecommendation, now time.Time) (int32, error) {
	for _, r := range recommendations {
		tz, err := time.LoadLocation(r.TimeZone)
		if err == nil {
			// if the timezone is invalid, just ignore it.
			now = now.In(tz)
		}

		if now.Hour() < r.To && now.Hour() >= r.From && (r.WeekDay == nil || now.Weekday().String() == *r.WeekDay) {
			return r.Value, nil
		}
	}
	return 0, errors.New("no recommendation slot")
}

// updateHPATargetValue updates the target value of the HPA.
// It looks for the corresponding metric (ContainerResource) and updates the target value.
func (c *Service) updateHPATargetValue(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName, targetValue int32) error {
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

		if targetValue-*m.ContainerResource.Target.AverageUtilization > int32(c.tortoiseHPATargetUtilizationMaxIncrease) {
			// We don't want to increase the target utilization that much because it might be dangerous.
			// (Reduce is OK)

			// We only allow to increase the target utilization by c.tortoiseHPATargetUtilizationMaxIncrease.
			maxIncrease := *m.ContainerResource.Target.AverageUtilization + int32(c.tortoiseHPATargetUtilizationMaxIncrease)
			m.ContainerResource.Target.AverageUtilization = &maxIncrease
			return nil
		}

		m.ContainerResource.Target.AverageUtilization = &targetValue

		return nil
	}

	return fmt.Errorf("no corresponding metric found: %s/%s", hpa.Namespace, hpa.Name)
}

func recordHPAMetric(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler) {
	for _, policies := range tortoise.Status.AutoscalingPolicy {
		for k, p := range policies.Policy {
			if p != autoscalingv1beta3.AutoscalingTypeHorizontal {
				continue
			}

			target, err := GetHPATargetValue(ctx, hpa, policies.ContainerName, k)
			if err != nil {
				log.FromContext(ctx).Error(err, "failed to get target value of the HPA", "hpa", klog.KObj(hpa))
				// ignore the error and go through all policies anyway.
				continue
			}

			metrics.ActualHPATargetUtilization.WithLabelValues(tortoise.Name, tortoise.Namespace, policies.ContainerName, k.String(), hpa.Name).Set(float64(target))
		}
	}

	metrics.ActualHPAMinReplicas.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(*hpa.Spec.MinReplicas))
	metrics.ActualHPAMaxReplicas.WithLabelValues(tortoise.Name, tortoise.Namespace, hpa.Name).Set(float64(hpa.Spec.MaxReplicas))
}

// GetHPATargetValue gets the target value of the HPA.
// It looks for the corresponding metric (ContainerResource) and gets the target value.
func GetHPATargetValue(ctx context.Context, hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName) (int32, error) {
	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ContainerResourceMetricSourceType {
			continue
		}

		if m.ContainerResource == nil {
			// shouldn't reach here
			log.FromContext(ctx).Error(nil, "invalid container resource metric", "hpa", klog.KObj(hpa))
			continue
		}

		if m.ContainerResource.Container != containerName || m.ContainerResource.Name != k || m.ContainerResource.Target.AverageUtilization == nil {
			continue
		}

		return *m.ContainerResource.Target.AverageUtilization, nil
	}

	return 0, fmt.Errorf("the metric for the container isn't found in the hpa: %s. (resource name: %s, container name: %s)", client.ObjectKeyFromObject(hpa).String(), k, containerName)
}

// excludeExternalMetric excludes the external metric from the HPA, based on the regex.
func (c *Service) excludeExternalMetric(ctx context.Context, hpa *v2.HorizontalPodAutoscaler) *v2.HorizontalPodAutoscaler {
	if c.externalMetricExclusionRegex == nil {
		// Do nothing.
		return hpa
	}
	newHPA := hpa.DeepCopy()
	newHPA.Spec.Metrics = []v2.MetricSpec{}
	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ExternalMetricSourceType {
			// No need to exclude.
			newHPA.Spec.Metrics = append(newHPA.Spec.Metrics, m)
			continue
		}

		if m.External == nil {
			// shouldn't reach here
			log.FromContext(ctx).Error(nil, "invalid external metric", "hpa", klog.KObj(hpa))

			// Keep it just in case.
			newHPA.Spec.Metrics = append(newHPA.Spec.Metrics, m)
			continue
		}

		if c.externalMetricExclusionRegex.MatchString(m.External.Metric.Name) {
			// Exclude
			log.FromContext(ctx).Info("exclude external metric", "hpa", klog.KObj(hpa), "excluded metric", m.External.Metric.Name)
			continue
		}
		// Not match = keep it
		newHPA.Spec.Metrics = append(newHPA.Spec.Metrics, m)
	}

	return newHPA
}
