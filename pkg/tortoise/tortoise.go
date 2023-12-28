package tortoise

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/event"
)

const tortoiseFinalizer = "tortoise.autoscaling.mercari.com/finalizer"

type Service struct {
	c        client.Client
	recorder record.EventRecorder

	rangeOfMinMaxReplicasRecommendationHour int
	timeZone                                *time.Location
	tortoiseUpdateInterval                  time.Duration
	// GatheringDataPeriodType means how long do we gather data for minReplica/maxReplica or data from VPA. "daily" and "weekly" are only valid value.
	// If "day", tortoise will consider all workload behaves very similarly every day.
	// If your workload may behave differently on, for example, weekdays and weekends, set this to "weekly".
	gatheringDataDuration string

	mu sync.RWMutex
	// TODO: Instead of here, we should store the last time of each tortoise in the status of the tortoise.
	lastTimeUpdateTortoise map[client.ObjectKey]time.Time
}

func New(c client.Client, recorder record.EventRecorder, rangeOfMinMaxReplicasRecommendationHour int, timeZone string, tortoiseUpdateInterval time.Duration, gatheringDataDuration string) (*Service, error) {
	jst, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, fmt.Errorf("load location: %w", err)
	}
	if gatheringDataDuration == "" {
		gatheringDataDuration = "weekly"
	}

	return &Service{
		c: c,

		recorder:                                recorder,
		rangeOfMinMaxReplicasRecommendationHour: rangeOfMinMaxReplicasRecommendationHour,
		gatheringDataDuration:                   gatheringDataDuration,
		timeZone:                                jst,
		tortoiseUpdateInterval:                  tortoiseUpdateInterval,
		lastTimeUpdateTortoise:                  map[client.ObjectKey]time.Time{},
	}, nil
}

func (s *Service) ShouldReconcileTortoiseNow(tortoise *v1beta3.Tortoise, now time.Time) (bool, time.Duration) {
	if tortoise.Spec.UpdateMode == v1beta3.UpdateModeEmergency && tortoise.Status.TortoisePhase != v1beta3.TortoisePhaseEmergency {
		// Tortoise which is emergency mode, but hasn't been handled by the controller yet. It should be updated ASAP.
		return true, 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	lastTime, ok := s.lastTimeUpdateTortoise[client.ObjectKeyFromObject(tortoise)]
	if !ok || lastTime.Add(s.tortoiseUpdateInterval).Before(now) {
		return true, 0
	}
	return false, lastTime.Add(s.tortoiseUpdateInterval).Sub(now)
}

func (s *Service) UpdateTortoisePhase(tortoise *v1beta3.Tortoise, now time.Time) *v1beta3.Tortoise {
	switch tortoise.Status.TortoisePhase {
	case "":
		tortoise = s.initializeTortoise(tortoise, now)
		r := "1 week"
		if s.gatheringDataDuration == "daily" {
			r = "1 day"
		}
		s.recorder.Event(tortoise, corev1.EventTypeNormal, event.Initialized, fmt.Sprintf("Tortoise is initialized and starts to gather data to make recommendations. It will take %s to finish gathering data and then tortoise starts to work actually", r))

	case v1beta3.TortoisePhaseInitializing:
		// change it to GatheringData anyway. Later the controller may change it back to initialize if VPA isn't ready.
		tortoise.Status.TortoisePhase = v1beta3.TortoisePhaseGatheringData
	case v1beta3.TortoisePhaseGatheringData:
		tortoise = s.changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(tortoise, now)
		if tortoise.Status.TortoisePhase == v1beta3.TortoisePhaseWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.Working, "Tortoise finishes gathering data and it starts to work on autoscaling")
		}
		if tortoise.Status.TortoisePhase == v1beta3.TortoisePhasePartlyWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.PartlyWorking, "Tortoise finishes gathering data in some metrics and it starts to work on autoscaling for those metrics. But some metrics are still gathering data")
		}
	case v1beta3.TortoisePhasePartlyWorking:
		tortoise = s.changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(tortoise, now)
		if tortoise.Status.TortoisePhase == v1beta3.TortoisePhaseWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.Working, "Tortoise finishes gathering data and it starts to work on autoscaling")
		}
	case v1beta3.TortoisePhaseEmergency:
		if tortoise.Spec.UpdateMode != v1beta3.UpdateModeEmergency {
			// Emergency mode is turned off.
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.Working, "Emergency mode is turned off. Tortoise starts to work on autoscaling normally. HPA.Spec.MinReplica will gradually be reduced")
			tortoise.Status.TortoisePhase = v1beta3.TortoisePhaseBackToNormal
		}
	}

	if tortoise.Spec.UpdateMode == v1beta3.UpdateModeEmergency {
		if tortoise.Status.TortoisePhase != v1beta3.TortoisePhaseEmergency {
			if !hasHorizontal(tortoise) {
				s.recorder.Event(tortoise, corev1.EventTypeWarning, event.EmergencyModeFailed, "Tortoise cannot move to Emergency mode because it doesn't have any horizontal autoscaling policy")
			} else if tortoise.Status.TortoisePhase != v1beta3.TortoisePhasePartlyWorking && tortoise.Status.TortoisePhase != v1beta3.TortoisePhaseWorking {
				s.recorder.Event(tortoise, corev1.EventTypeWarning, event.EmergencyModeFailed, "Tortoise cannot move to Emergency mode because it doesn't have enough historical data to increase the number of replicas")
			} else {
				s.recorder.Event(tortoise, corev1.EventTypeNormal, event.EmergencyModeEnabled, "Tortoise is in Emergency mode. It will increase the number of replicas")
				tortoise.Status.TortoisePhase = v1beta3.TortoisePhaseEmergency
			}
		}
	}

	return tortoise
}

func hasHorizontal(tortoise *v1beta3.Tortoise) bool {
	for _, r := range tortoise.Status.AutoscalingPolicy {
		for _, p := range r.Policy {
			if p == v1beta3.AutoscalingTypeHorizontal {
				return true
			}
		}
	}
	return false
}

func (s *Service) changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(tortoise *v1beta3.Tortoise, now time.Time) *v1beta3.Tortoise {
	for _, r := range tortoise.Status.Recommendations.Horizontal.MinReplicas {
		if r.Value == 0 {
			return tortoise
		}
	}
	for _, r := range tortoise.Status.Recommendations.Horizontal.MaxReplicas {
		if r.Value == 0 {
			return tortoise
		}
	}

	// Until MaxReplicas/MinReplicas recommendation are ready,
	// we never set TortoisePhase to Working or PartlyWorking.

	someAreGathering := false
	someAreWorking := false
	for i, c := range tortoise.Status.ContainerResourcePhases {
		for rn, p := range c.ResourcePhases {
			if p.Phase == v1beta3.ContainerResourcePhaseOff {
				// ignore
				continue
			}
			if p.Phase == v1beta3.ContainerResourcePhaseWorking {
				someAreWorking = true
				continue
			}
			// If the last transition time is within 1 week, we consider it's still gathering data.
			gatheringPeriod := 7 * 24 * time.Hour
			if s.gatheringDataDuration == "daily" {
				gatheringPeriod = 24 * time.Hour
			}
			if p.LastTransitionTime.Add(gatheringPeriod).After(now) {
				someAreGathering = true
			} else {
				// It's finish gathering data.
				tortoise.Status.ContainerResourcePhases[i].ResourcePhases[rn] = v1beta3.ResourcePhase{
					Phase:              v1beta3.ContainerResourcePhaseWorking,
					LastTransitionTime: metav1.NewTime(now),
				}
				someAreWorking = true
			}
		}
	}

	if someAreGathering && someAreWorking {
		// Some are working, but some are still gathering data.
		tortoise.Status.TortoisePhase = v1beta3.TortoisePhasePartlyWorking
	} else if !someAreGathering && someAreWorking {
		// All are working.
		tortoise.Status.TortoisePhase = v1beta3.TortoisePhaseWorking
	}

	return tortoise
}

func (s *Service) initializeMinMaxReplicas(tortoise *v1beta3.Tortoise) *v1beta3.Tortoise {
	recommendations := []v1beta3.ReplicasRecommendation{}
	from := 0
	to := s.rangeOfMinMaxReplicasRecommendationHour
	weekDay := time.Sunday
	for {
		if s.gatheringDataDuration == "daily" {
			recommendations = append(recommendations, v1beta3.ReplicasRecommendation{
				From:     from,
				To:       to,
				TimeZone: s.timeZone.String(),
			})
		} else if s.gatheringDataDuration == "weekly" {
			recommendations = append(recommendations, v1beta3.ReplicasRecommendation{
				From:     from,
				To:       to,
				TimeZone: s.timeZone.String(),
				WeekDay:  pointer.String(weekDay.String()),
			})
		}

		if to == 24 {
			if weekDay == time.Saturday || s.gatheringDataDuration == "daily" {
				break
			}

			weekDay += 1
			from = 0
			to = s.rangeOfMinMaxReplicasRecommendationHour
			continue
		}
		from += s.rangeOfMinMaxReplicasRecommendationHour
		to += s.rangeOfMinMaxReplicasRecommendationHour
	}
	tortoise.Status.Recommendations.Horizontal.MinReplicas = recommendations
	tortoise.Status.Recommendations.Horizontal.MaxReplicas = recommendations

	return tortoise
}

func (s *Service) initializeTortoise(tortoise *v1beta3.Tortoise, now time.Time) *v1beta3.Tortoise {
	tortoise = s.initializeMinMaxReplicas(tortoise)
	tortoise.Status.TortoisePhase = v1beta3.TortoisePhaseInitializing

	tortoise.Status.Conditions.ContainerRecommendationFromVPA = make([]v1beta3.ContainerRecommendationFromVPA, len(tortoise.Status.AutoscalingPolicy))
	for i, c := range tortoise.Status.AutoscalingPolicy {
		tortoise.Status.Conditions.ContainerRecommendationFromVPA[i] = v1beta3.ContainerRecommendationFromVPA{
			ContainerName: c.ContainerName,
			Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
			MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
		}
	}
	tortoise.Status.Targets.ScaleTargetRef = tortoise.Spec.TargetRefs.ScaleTargetRef

	for _, p := range tortoise.Status.AutoscalingPolicy {
		phase := v1beta3.ContainerResourcePhases{
			ContainerName:  p.ContainerName,
			ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{},
		}
		for rn, policy := range p.Policy {
			if policy == v1beta3.AutoscalingTypeOff {
				phase.ResourcePhases[rn] = v1beta3.ResourcePhase{
					Phase:              v1beta3.ContainerResourcePhaseOff,
					LastTransitionTime: metav1.NewTime(now),
				}
				continue
			}

			phase.ResourcePhases[rn] = v1beta3.ResourcePhase{
				Phase:              v1beta3.ContainerResourcePhaseGatheringData,
				LastTransitionTime: metav1.NewTime(now),
			}
		}
		tortoise.Status.ContainerResourcePhases = append(tortoise.Status.ContainerResourcePhases, phase)
	}

	return tortoise.DeepCopy()
}

func (s *Service) UpdateUpperRecommendation(tortoise *v1beta3.Tortoise, vpa *v1.VerticalPodAutoscaler) *v1beta3.Tortoise {
	upperMap := make(map[string]map[corev1.ResourceName]resource.Quantity, len(vpa.Status.Recommendation.ContainerRecommendations))
	for _, c := range vpa.Status.Recommendation.ContainerRecommendations {
		upperMap[c.ContainerName] = make(map[corev1.ResourceName]resource.Quantity, len(c.UpperBound))
		for rn, r := range c.UpperBound {
			upperMap[c.ContainerName][rn] = r
		}
	}

	targetMap := make(map[string]map[corev1.ResourceName]resource.Quantity, len(vpa.Status.Recommendation.ContainerRecommendations))
	for _, c := range vpa.Status.Recommendation.ContainerRecommendations {
		targetMap[c.ContainerName] = make(map[corev1.ResourceName]resource.Quantity, len(c.UpperBound))
		for rn, r := range c.Target {
			targetMap[c.ContainerName][rn] = r
		}
	}

	for k, r := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
		for rn, max := range r.MaxRecommendation {
			currentUpperFromVPA := upperMap[r.ContainerName][rn]
			currentTargetFromVPA := targetMap[r.ContainerName][rn]
			currentMaxRecommendation := max.Quantity

			rq := v1beta3.ResourceQuantity{
				Quantity:  currentTargetFromVPA,
				UpdatedAt: metav1.Now(),
			}

			// Always replace Recommendation with the current recommendation.
			tortoise.Status.Conditions.ContainerRecommendationFromVPA[k].Recommendation[rn] = rq

			// And then, check if we need to replace MaxRecommendation with the current recommendation.
			if currentMaxRecommendation.Cmp(currentTargetFromVPA) > 0 && currentMaxRecommendation.Cmp(currentUpperFromVPA) < 0 {
				// currentTargetFromVPA < currentMaxRecommendation < currentUpperFromVPA

				// This case, recommendation is in the acceptable range. We don't update maxRecommendation.
				continue
			}

			// replace with currentTargetFromVPA if:
			// currentMaxRecommendation < currentTargetFromVPA: currentMaxRecommendation is too small based on the currentTargetFromVPA.
			// OR
			// currentUpperFromVPA < currentMaxRecommendation: currentMaxRecommendation is too big based on the currentUpperFromVPA.

			tortoise.Status.Conditions.ContainerRecommendationFromVPA[k].MaxRecommendation[rn] = rq
		}
	}
	return tortoise
}

func (s *Service) GetTortoise(ctx context.Context, namespacedName types.NamespacedName) (*v1beta3.Tortoise, error) {
	t := &v1beta3.Tortoise{}
	if err := s.c.Get(ctx, namespacedName, t); err != nil {
		return nil, fmt.Errorf("failed to get tortoise: %w", err)
	}
	return t, nil
}

func (s *Service) AddFinalizer(ctx context.Context, tortoise *v1beta3.Tortoise) (*v1beta3.Tortoise, error) {
	if controllerutil.ContainsFinalizer(tortoise, tortoiseFinalizer) {
		return tortoise, nil
	}

	updateFn := func() error {
		retTortoise := &v1beta3.Tortoise{}
		err := s.c.Get(ctx, client.ObjectKeyFromObject(tortoise), retTortoise)
		if err != nil {
			return err
		}
		controllerutil.AddFinalizer(retTortoise, tortoiseFinalizer)
		return s.c.Update(ctx, retTortoise)
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, updateFn)
	if err != nil {
		return tortoise, fmt.Errorf("failed to add finalizer: %w", err)
	}

	return tortoise, nil
}

func (s *Service) RemoveFinalizer(ctx context.Context, tortoise *v1beta3.Tortoise) error {
	if !controllerutil.ContainsFinalizer(tortoise, tortoiseFinalizer) {
		return nil
	}

	updateFn := func() error {
		retTortoise := &v1beta3.Tortoise{}
		err := s.c.Get(ctx, client.ObjectKeyFromObject(tortoise), retTortoise)
		if err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(tortoise, tortoiseFinalizer)
		return s.c.Update(ctx, tortoise)
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, updateFn)
	if err != nil {
		return fmt.Errorf("failed to remove finalizer: %w", err)
	}
	return nil
}

func (s *Service) UpdateTortoiseStatus(ctx context.Context, originalTortoise *v1beta3.Tortoise, now time.Time, timeRecord bool) (*v1beta3.Tortoise, error) {
	logger := log.FromContext(ctx)
	logger.Info("update tortoise status", "tortoise", klog.KObj(originalTortoise))
	retTortoise := &v1beta3.Tortoise{}
	updateFn := func() error {
		retTortoise = &v1beta3.Tortoise{}
		err := s.c.Get(ctx, client.ObjectKeyFromObject(originalTortoise), retTortoise)
		if err != nil {
			return fmt.Errorf("get tortoise to update status: %w", err)
		}
		// It should be OK to overwrite the status, because the controller is the only person to update it.
		retTortoise.Status = originalTortoise.Status

		err = s.c.Status().Update(ctx, retTortoise)
		if err != nil {
			return fmt.Errorf("update tortoise status: %w", err)
		}
		return nil
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, updateFn)
	if err != nil {
		return originalTortoise, err
	}

	if timeRecord {
		s.updateLastTimeUpdateTortoise(originalTortoise, now)
	}

	return originalTortoise, nil
}

func (s *Service) updateLastTimeUpdateTortoise(tortoise *v1beta3.Tortoise, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastTimeUpdateTortoise[client.ObjectKeyFromObject(tortoise)] = now
}

func (s *Service) RecordReconciliationFailure(t *v1beta3.Tortoise, err error, now time.Time) *v1beta3.Tortoise {
	if err != nil {
		s.recorder.Event(t, "Warning", "ReconcileError", err.Error())
		for i := range t.Status.Conditions.TortoiseConditions {
			if t.Status.Conditions.TortoiseConditions[i].Type == v1beta3.TortoiseConditionTypeFailedToReconcile {
				// TODO: have a clear reason and utilize it to have a better reconciliation next.
				// For example, in some cases, the reconciliation may keep failing until people fix some problems manually.
				t.Status.Conditions.TortoiseConditions[i].Reason = "ReconcileError"
				t.Status.Conditions.TortoiseConditions[i].Message = err.Error()
				t.Status.Conditions.TortoiseConditions[i].Status = corev1.ConditionTrue
				t.Status.Conditions.TortoiseConditions[i].LastTransitionTime = metav1.NewTime(now)
				t.Status.Conditions.TortoiseConditions[i].LastUpdateTime = metav1.NewTime(now)
				return t
			}
		}
		// add as a new condition if not found.
		t.Status.Conditions.TortoiseConditions = append(t.Status.Conditions.TortoiseConditions, v1beta3.TortoiseCondition{
			Type:               v1beta3.TortoiseConditionTypeFailedToReconcile,
			Status:             corev1.ConditionTrue,
			Reason:             "ReconcileError",
			Message:            err.Error(),
			LastTransitionTime: metav1.NewTime(now),
			LastUpdateTime:     metav1.NewTime(now),
		})
		return t
	}

	for i := range t.Status.Conditions.TortoiseConditions {
		if t.Status.Conditions.TortoiseConditions[i].Type == v1beta3.TortoiseConditionTypeFailedToReconcile {
			t.Status.Conditions.TortoiseConditions[i].Reason = ""
			t.Status.Conditions.TortoiseConditions[i].Message = ""
			t.Status.Conditions.TortoiseConditions[i].Status = corev1.ConditionFalse
			t.Status.Conditions.TortoiseConditions[i].LastTransitionTime = metav1.NewTime(now)
			t.Status.Conditions.TortoiseConditions[i].LastUpdateTime = metav1.NewTime(now)
			return t
		}
	}
	return t
}

type resourceNameAndContainerName struct {
	rn            corev1.ResourceName
	containerName string
}

// UpdateTortoiseAutoscalingPolicyInStatus updates .status.autoscalingPolicy based on the policy in .spec.autoscalingPolicy,
// and the existing container names in the workload.
func UpdateTortoiseAutoscalingPolicyInStatus(tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler) *v1beta3.Tortoise {
	if tortoise.Spec.AutoscalingPolicy != nil {
		// we just use the policy in the spec if it's non-empty.
		tortoise.Status.AutoscalingPolicy = tortoise.Spec.AutoscalingPolicy
		return tortoise
	}

	containerWOResourceRequest := sets.New[resourceNameAndContainerName]() // container names which doesn't have resource requests
	containerNames := sets.New[string]()                                   // all container names
	for _, r := range tortoise.Status.Conditions.ContainerResourceRequests {
		containerNames.Insert(r.ContainerName)
		if r.Resource.Cpu().Value() == 0 {
			containerWOResourceRequest.Insert(resourceNameAndContainerName{corev1.ResourceCPU, r.ContainerName})
		}
		if r.Resource.Memory().Value() == 0 {
			containerWOResourceRequest.Insert(resourceNameAndContainerName{corev1.ResourceMemory, r.ContainerName})
		}
	}

	// First, we checked the existing policies and the containers in the workload
	// so that we can remove the useless policies and add the lacking policies.
	containersWithPolicy := sets.New[string]()
	for _, p := range tortoise.Status.AutoscalingPolicy {
		containersWithPolicy.Insert(p.ContainerName)
	}

	uselessPolicis := containersWithPolicy.Difference(containerNames)
	for _, p := range uselessPolicis.UnsortedList() {
		for i, rp := range tortoise.Status.AutoscalingPolicy {
			if rp.ContainerName == p {
				tortoise.Status.AutoscalingPolicy = append(tortoise.Status.AutoscalingPolicy[:i], tortoise.Status.AutoscalingPolicy[i+1:]...)
				break
			}
		}
	}

	lackingPolicies := containerNames.Difference(containersWithPolicy)
	for _, p := range lackingPolicies.UnsortedList() {
		tortoise.Status.AutoscalingPolicy = append(tortoise.Status.AutoscalingPolicy, v1beta3.ContainerAutoscalingPolicy{
			ContainerName: p,
			Policy: map[corev1.ResourceName]v1beta3.AutoscalingType{
				corev1.ResourceCPU:    v1beta3.AutoscalingTypeHorizontal,
				corev1.ResourceMemory: v1beta3.AutoscalingTypeVertical,
			},
		})
	}

	// And, if the existing HPA is attached, we modify the policy for resources managed by the HPA to Horizontal.
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		hpaManagedResourceAndContainer := sets.New[resourceNameAndContainerName]()
		for _, m := range hpa.Spec.Metrics {
			if m.Type != v2.ContainerResourceMetricSourceType {
				continue
			}
			hpaManagedResourceAndContainer.Insert(resourceNameAndContainerName{m.ContainerResource.Name, m.ContainerResource.Container})
		}

		// If the existing HPA is attached, we sets “Horizontal” to resources managed by the attached HPA by default.
		for i := range tortoise.Status.AutoscalingPolicy {
			if hpaManagedResourceAndContainer.Has(resourceNameAndContainerName{corev1.ResourceCPU, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
				// If HPA has the metrics for this container's CPU, we set Horizontal.
				tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] = v1beta3.AutoscalingTypeHorizontal
			} else {
				// Otherwise, set Vertical.
				tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] = v1beta3.AutoscalingTypeVertical
			}
			if hpaManagedResourceAndContainer.Has(resourceNameAndContainerName{corev1.ResourceMemory, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
				// If HPA has the metrics for this container's memory , we set Horizontal.
				tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] = v1beta3.AutoscalingTypeHorizontal
			} else {
				// Otherwise, set Vertical.
				tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] = v1beta3.AutoscalingTypeVertical
			}
		}
	}

	// If the container doesn't have resource request, we set the policy to Off because we couldn't make a recommendation.
	for i := range tortoise.Status.AutoscalingPolicy {
		if containerWOResourceRequest.Has(resourceNameAndContainerName{corev1.ResourceCPU, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
			tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] = v1beta3.AutoscalingTypeOff
		}
		if containerWOResourceRequest.Has(resourceNameAndContainerName{corev1.ResourceMemory, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
			tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] = v1beta3.AutoscalingTypeOff
		}
	}

	// sort the autoscaling policy by container name.
	sort.Slice(tortoise.Status.AutoscalingPolicy, func(i, j int) bool {
		return tortoise.Status.AutoscalingPolicy[i].ContainerName < tortoise.Status.AutoscalingPolicy[j].ContainerName
	})

	return tortoise
}
