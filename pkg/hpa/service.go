package hpa

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1beta2 "github.com/mercari/tortoise/api/v1beta2"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/metrics"
)

type Service struct {
	c client.Client

	replicaReductionFactor                  float64
	upperTargetResourceUtilization          int32
	tortoiseHPATargetUtilizationMaxIncrease int
	recorder                                record.EventRecorder
}

func New(c client.Client, recorder record.EventRecorder, replicaReductionFactor float64, upperTargetResourceUtilization, tortoiseHPATargetUtilizationMaxIncrease int) *Service {
	return &Service{
		c:                                       c,
		replicaReductionFactor:                  replicaReductionFactor,
		upperTargetResourceUtilization:          int32(upperTargetResourceUtilization),
		tortoiseHPATargetUtilizationMaxIncrease: tortoiseHPATargetUtilizationMaxIncrease,
		recorder:                                recorder,
	}
}

func (c *Service) InitializeHPA(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise, dm *v1.Deployment, now time.Time) (*autoscalingv1beta2.Tortoise, error) {
	logger := log.FromContext(ctx)
	// if all policy is off or Vertical, we don't need HPA.
	if !HasHorizontal(tortoise) {
		logger.V(4).Info("no horizontal policy, no need to create HPA")
		return tortoise, nil
	}

	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		logger.V(4).Info("user specified the existing HPA, no need to create HPA")

		// update the existing HPA that the user set on tortoise.
		tortoise, err := c.giveAnnotationsOnExistingHPA(ctx, tortoise)
		if err != nil {
			return tortoise, fmt.Errorf("give annotations on a hpa specified in targetrefs: %w", err)
		}

		c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPAUpdated", fmt.Sprintf("Updated HPA %s/%s", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

		return tortoise, nil
	}
	logger.V(4).Info("no existing HPA specified, creating HPA")

	// create default HPA.
	_, tortoise, err := c.CreateHPA(ctx, tortoise, dm, now)
	if err != nil {
		return tortoise, fmt.Errorf("create hpa: %w", err)
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPACreated", fmt.Sprintf("Initialized a HPA %s/%s for a created tortoise", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

	return tortoise, nil
}

func (c *Service) giveAnnotationsOnExistingHPA(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise) (*autoscalingv1beta2.Tortoise, error) {
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
		return c.c.Update(ctx, hpa)
	}
	tortoise.Status.Targets.HorizontalPodAutoscaler = *tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName

	return tortoise, retry.RetryOnConflict(retry.DefaultRetry, updateFn)
}

func (c *Service) DeleteHPACreatedByTortoise(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise) error {
	if tortoise.Spec.DeletionPolicy == autoscalingv1beta2.DeletionPolicyNoDelete {
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

type resourceNameAndContainerName struct {
	rn            corev1.ResourceName
	containerName string
}

// addHPAMetricsFromTortoiseAutoscalingPolicy adds metrics to the HPA based on the autoscaling policy in the tortoise.
// Note that it doesn't update the HPA in kube-apiserver, you have to do that after this function.
func (c *Service) addHPAMetricsFromTortoiseAutoscalingPolicy(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise, currenthpa *v2.HorizontalPodAutoscaler, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta2.Tortoise, bool) {
	hpaEdited := false

	policies := sets.New[string]()
	horizontalResourceAndContainer := sets.New[resourceNameAndContainerName]()
	for _, p := range tortoise.Spec.ResourcePolicy {
		policies.Insert(p.ContainerName)
		for rn, ap := range p.AutoscalingPolicy {
			if ap == autoscalingv1beta2.AutoscalingTypeHorizontal {
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
					AverageUtilization: pointer.Int32(50),
				},
			},
		}
		currenthpa.Spec.Metrics = append(currenthpa.Spec.Metrics, m)
		hpaEdited = true
		found := false
		for i, p := range tortoise.Status.ContainerResourcePhases {
			if p.ContainerName == d.containerName {
				tortoise.Status.ContainerResourcePhases[i].ResourcePhases[d.rn] = autoscalingv1beta2.ResourcePhase{
					Phase:              autoscalingv1beta2.ContainerResourcePhaseGatheringData,
					LastTransitionTime: metav1.NewTime(now),
				}

				found = true
				break
			}
		}
		if !found {
			tortoise.Status.ContainerResourcePhases = append(tortoise.Status.ContainerResourcePhases, autoscalingv1beta2.ContainerResourcePhases{
				ContainerName: d.containerName,
				ResourcePhases: map[corev1.ResourceName]autoscalingv1beta2.ResourcePhase{
					d.rn: {
						Phase:              autoscalingv1beta2.ContainerResourcePhaseGatheringData,
						LastTransitionTime: metav1.NewTime(now),
					},
				},
			})
		}
	}

	// remove metrics
	newMetrics := []v2.MetricSpec{}
	for _, m := range currenthpa.Spec.Metrics {
		if m.Type != v2.ContainerResourceMetricSourceType {
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

func (c *Service) CreateHPA(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise, dm *v1.Deployment, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta2.Tortoise, error) {
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
			Name:      autoscalingv1beta2.TortoiseDefaultHPAName(tortoise.Name),
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

	hpa, tortoise, _ = c.addHPAMetricsFromTortoiseAutoscalingPolicy(ctx, tortoise, hpa, now)

	tortoise.Status.Targets.HorizontalPodAutoscaler = hpa.Name

	err := c.c.Create(ctx, hpa)
	return hpa.DeepCopy(), tortoise, err
}

func (c *Service) GetHPAOnTortoise(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise) (*v2.HorizontalPodAutoscaler, error) {
	if !HasHorizontal(tortoise) {
		// there should be no HPA
		return nil, nil
	}
	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
		return nil, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}
	return hpa, nil
}

func (c *Service) ChangeHPAFromTortoiseRecommendation(tortoise *autoscalingv1beta2.Tortoise, hpa *v2.HorizontalPodAutoscaler, now time.Time, recordMetrics bool) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta2.Tortoise, error) {
	readyHorizontalResourceAndContainer := sets.New[resourceNameAndContainerName]()
	for _, p := range tortoise.Spec.ResourcePolicy {
		for rn, ap := range p.AutoscalingPolicy {
			if ap == autoscalingv1beta2.AutoscalingTypeHorizontal {
				readyHorizontalResourceAndContainer.Insert(resourceNameAndContainerName{rn, p.ContainerName})
			}
		}
	}
	for _, p := range tortoise.Status.ContainerResourcePhases {
		for rn, phase := range p.ResourcePhases {
			if phase.Phase != autoscalingv1beta2.ContainerResourcePhaseWorking {
				readyHorizontalResourceAndContainer.Delete(resourceNameAndContainerName{rn, p.ContainerName})
			}
		}
	}

	for _, t := range tortoise.Status.Recommendations.Horizontal.TargetUtilizations {
		for resourcename, proposedTarget := range t.TargetUtilization {
			if !readyHorizontalResourceAndContainer.Has(resourceNameAndContainerName{resourcename, t.ContainerName}) {
				// this recommendation is not ready. We don't want to apply it.
				continue
			}

			metrics.ProposedHPATargetUtilization.WithLabelValues(tortoise.Name, tortoise.Namespace, t.ContainerName, resourcename.String(), hpa.Name).Set(float64(proposedTarget))
			if err := c.updateHPATargetValue(hpa, t.ContainerName, resourcename, proposedTarget); err != nil {
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
	case autoscalingv1beta2.TortoisePhaseEmergency:
		// when emergency mode, we set the same value on minReplicas.
		min = max
	case autoscalingv1beta2.TortoisePhaseBackToNormal:
		idealMin, err := GetReplicasRecommendation(tortoise.Status.Recommendations.Horizontal.MinReplicas, now)
		if err != nil {
			return nil, tortoise, fmt.Errorf("get minReplicas recommendation: %w", err)
		}
		currentMin := *hpa.Spec.MinReplicas
		reduced := int32(math.Trunc(float64(currentMin) * c.replicaReductionFactor))
		if idealMin > reduced {
			min = idealMin
			// BackToNormal is finished
			tortoise.Status.TortoisePhase = autoscalingv1beta2.TortoisePhaseWorking
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

func (c *Service) UpdateHPASpecFromTortoiseAutoscalingPolicy(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise, dm *v1.Deployment, now time.Time) (*autoscalingv1beta2.Tortoise, error) {
	if !HasHorizontal(tortoise) {
		err := c.DeleteHPACreatedByTortoise(ctx, tortoise)
		if err != nil && !apierrors.IsNotFound(err) {
			return tortoise, fmt.Errorf("delete hpa created by tortoise: %w", err)
		}
		// No need to edit container resource phase.

		c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPADeleted", fmt.Sprintf("Deleted a HPA %s/%s because tortoise has no resource to scale horizontally", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

		return tortoise, nil
	}

	hpa := &v2.HorizontalPodAutoscaler{}
	if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
		if apierrors.IsNotFound(err) {
			// If not found, it's one of:
			// - the user didn't specify Horizontal in any autoscalingPolicy previously,
			//   but just updated tortoise to have Horizontal in some.
			//   - In that case, we need to create an initial HPA.
			tortoise, err = c.InitializeHPA(ctx, tortoise, dm, now)
			if err != nil {
				return tortoise, fmt.Errorf("initialize hpa: %w", err)
			}

			c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPACreated", fmt.Sprintf("Initialized a HPA %s/%s because tortoise has resource to scale horizontally", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))
			return tortoise, nil
		}
		return tortoise, fmt.Errorf("failed to get hpa on tortoise: %w", err)
	}

	// make sure it's managed by tortoise
	if v, ok := hpa.Annotations[annotation.ManagedByTortoiseAnnotation]; !ok || v != "true" {
		return tortoise, fmt.Errorf("the HPA %s/%s is specified in tortoise, but not managed by tortoise", hpa.Namespace, hpa.Name)
	}

	var newhpa *v2.HorizontalPodAutoscaler
	var isHpaEdited bool
	newhpa, tortoise, isHpaEdited = c.addHPAMetricsFromTortoiseAutoscalingPolicy(ctx, tortoise, hpa, now)
	if !isHpaEdited {
		// User didn't change anything.
		return tortoise, nil
	}

	updateFn := func() error {
		hpa := &v2.HorizontalPodAutoscaler{}
		if err := c.c.Get(ctx, types.NamespacedName{Namespace: tortoise.Namespace, Name: tortoise.Status.Targets.HorizontalPodAutoscaler}, hpa); err != nil {
			return fmt.Errorf("failed to get hpa on tortoise: %w", err)
		}

		hpa.Spec.Metrics = newhpa.Spec.Metrics

		return c.c.Update(ctx, newhpa)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return tortoise, err
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPAUpdated", fmt.Sprintf("Updated a HPA %s/%s because the autoscaling policy is changed in the tortoise", tortoise.Namespace, tortoise.Status.Targets.HorizontalPodAutoscaler))

	return tortoise, nil
}

func HasHorizontal(tortoise *autoscalingv1beta2.Tortoise) bool {
	for _, r := range tortoise.Spec.ResourcePolicy {
		for _, p := range r.AutoscalingPolicy {
			if p == autoscalingv1beta2.AutoscalingTypeHorizontal {
				return true
			}
		}
	}
	return false
}

func (c *Service) UpdateHPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1beta2.Tortoise, now time.Time) (*v2.HorizontalPodAutoscaler, *autoscalingv1beta2.Tortoise, error) {
	// if all policy is off or Vertical, we don't update HPA.
	if !HasHorizontal(tortoise) {
		return nil, tortoise, nil
	}

	retTortoise := &autoscalingv1beta2.Tortoise{}
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
		if tortoise.Spec.UpdateMode == autoscalingv1beta2.UpdateModeOff {
			// don't update status if update mode is off. (= dryrun)
			return nil
		}
		retHPA = hpa
		return c.c.Update(ctx, hpa)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return nil, nil, err
	}

	if tortoise.Spec.UpdateMode != autoscalingv1beta2.UpdateModeOff {
		c.recorder.Event(tortoise, corev1.EventTypeNormal, "HPAUpdated", fmt.Sprintf("HPA %s/%s is updated by the recommendation", retHPA.Namespace, retHPA.Name))
	}

	return retHPA, retTortoise, nil
}

// GetReplicasRecommendation finds the corresponding recommendations.
func GetReplicasRecommendation(recommendations []autoscalingv1beta2.ReplicasRecommendation, now time.Time) (int32, error) {
	for _, r := range recommendations {
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
