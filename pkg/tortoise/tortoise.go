package tortoise

import (
	"context"
	"fmt"
	"reflect"
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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/event"
	"github.com/mercari/tortoise/pkg/metrics"
	"github.com/mercari/tortoise/pkg/utils"
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

func initializeContainerResourcePhase(tortoise *v1beta3.Tortoise, now time.Time) *v1beta3.Tortoise {
	for _, c := range tortoise.Status.AutoscalingPolicy {
		for rn := range c.Policy {
			// set all to gathering data
			utils.ChangeTortoiseContainerResourcePhase(tortoise, c.ContainerName, rn, now, v1beta3.ContainerResourcePhaseGatheringData)
		}
	}

	return tortoise
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
		tortoise = initializeContainerResourcePhase(tortoise, now)
	case v1beta3.TortoisePhaseGatheringData:
		tortoise = s.changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(tortoise, now)
		if tortoise.Status.TortoisePhase == v1beta3.TortoisePhaseWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.Working, "Tortoise finishes gathering data and it starts to work on autoscaling")
		}
		if tortoise.Status.TortoisePhase == v1beta3.TortoisePhasePartlyWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.PartlyWorking, "Tortoise finishes gathering data in some metrics and it starts to work on autoscaling for those metrics. But some metrics are still gathering data")
		}
	case v1beta3.TortoisePhaseWorking:
		// Some resource may be changed to gathering data, if autoscaling policy is changed.
		tortoise = s.changeTortoisePhaseWorkingIfTortoiseFinishedGatheringData(tortoise, now)
		if tortoise.Status.TortoisePhase == v1beta3.TortoisePhasePartlyWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, event.PartlyWorking, "Tortoise was fully working, but needs to collect some date for some resources")
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
	// If recommendation of maxReplicas or minReplicas is 0, it means horizontal autoscaling is not ready yet.
	horizontalUnready := false
	for _, r := range tortoise.Status.Recommendations.Horizontal.MinReplicas {
		if r.Value == 0 {
			horizontalUnready = true
		}
	}
	for _, r := range tortoise.Status.Recommendations.Horizontal.MaxReplicas {
		if r.Value == 0 {
			horizontalUnready = true
		}
	}

	if horizontalUnready {
		for _, c := range tortoise.Status.AutoscalingPolicy {
			for rn, p := range c.Policy {
				if p == v1beta3.AutoscalingTypeHorizontal {
					utils.ChangeTortoiseContainerResourcePhase(tortoise, c.ContainerName, rn, now, v1beta3.ContainerResourcePhaseGatheringData)
				}
			}
		}
	}

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
	} else if someAreGathering && !someAreWorking {
		// All are gathering data.
		tortoise.Status.TortoisePhase = v1beta3.TortoisePhaseGatheringData
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
				WeekDay:  ptr.To(weekDay.String()),
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
		for rn, policy := range p.Policy {
			if policy == v1beta3.AutoscalingTypeOff {
				utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, rn, now, v1beta3.ContainerResourcePhaseOff)
				continue
			}

			utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, rn, now, v1beta3.ContainerResourcePhaseGatheringData)
		}
	}

	return tortoise.DeepCopy()
}

// SyncContainerRecommendationFromVPA makes sure that ContainerRecommendationFromVPA has all containers
func (s *Service) syncContainerRecommendationFromVPA(tortoise *v1beta3.Tortoise) *v1beta3.Tortoise {
	containerNames := sets.New[string]()
	for _, c := range tortoise.Status.AutoscalingPolicy {
		// check the containers from the autoscaling policy.
		containerNames.Insert(c.ContainerName)
	}

	containersInContainerRecommendationFromVPA := sets.New[string]()
	for _, r := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
		containersInContainerRecommendationFromVPA.Insert(r.ContainerName)
	}

	containersToAdd := containerNames.Difference(containersInContainerRecommendationFromVPA).UnsortedList()
	sort.Slice(containersToAdd, func(i, j int) bool {
		return containersToAdd[i] < containersToAdd[j]
	})
	containersToRemove := containersInContainerRecommendationFromVPA.Difference(containerNames)
	for _, c := range containersToAdd {
		// Containers are not included in tortoise.Status.Conditions.ContainerRecommendationFromVPA, probably new containers.
		tortoise.Status.Conditions.ContainerRecommendationFromVPA = append(tortoise.Status.Conditions.ContainerRecommendationFromVPA, v1beta3.ContainerRecommendationFromVPA{
			ContainerName: c,
			Recommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
			MaxRecommendation: map[corev1.ResourceName]v1beta3.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
		})
	}

	// remove containersToRemove from tortoise.Status.Conditions.ContainerRecommendationFromVPA
	for _, c := range containersToRemove.UnsortedList() {
		for i, r := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
			if r.ContainerName == c {
				tortoise.Status.Conditions.ContainerRecommendationFromVPA = append(tortoise.Status.Conditions.ContainerRecommendationFromVPA[:i], tortoise.Status.Conditions.ContainerRecommendationFromVPA[i+1:]...)
				break
			}
		}
	}

	return tortoise
}

func (s *Service) UpdateContainerRecommendationFromVPA(tortoise *v1beta3.Tortoise, vpa *v1.VerticalPodAutoscaler, now time.Time) *v1beta3.Tortoise {
	tortoise = s.syncContainerRecommendationFromVPA(tortoise)

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
				UpdatedAt: metav1.NewTime(now),
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

func (s *Service) ListTortoise(ctx context.Context, namespaceName string) (*v1beta3.TortoiseList, error) {
	tl := &v1beta3.TortoiseList{}
	if err := s.c.List(ctx, tl, &client.ListOptions{
		Namespace: namespaceName,
	}); err != nil {
		return nil, fmt.Errorf("failed to list tortoise in %s: %w", namespaceName, err)
	}
	return tl, nil
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
	retried := -1
	updateFn := func() error {
		retried++ // To debug how many times it retried.
		retTortoise = &v1beta3.Tortoise{}
		err := s.c.Get(ctx, client.ObjectKeyFromObject(originalTortoise), retTortoise)
		if err != nil {
			return fmt.Errorf("get tortoise to update status: %w", err)
		}
		// It should be OK to overwrite the status, because the controller is the only person to update it.
		retTortoise.Status = originalTortoise.Status

		err = s.c.Status().Update(ctx, retTortoise)
		if err != nil {
			return err
		}
		return nil
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, updateFn)
	if err != nil {
		return originalTortoise, fmt.Errorf("failed to update tortoise status (retried: %v): %w", retried, err)
	}

	if timeRecord {
		s.updateLastTimeUpdateTortoise(originalTortoise, now)
	}

	return retTortoise, nil
}

func (s *Service) updateLastTimeUpdateTortoise(tortoise *v1beta3.Tortoise, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastTimeUpdateTortoise[client.ObjectKeyFromObject(tortoise)] = now
}

func (s *Service) RecordReconciliationFailure(t *v1beta3.Tortoise, err error, now time.Time) *v1beta3.Tortoise {
	if err != nil {
		s.recorder.Event(t, "Warning", "ReconcileError", err.Error())
		return utils.ChangeTortoiseCondition(t, v1beta3.TortoiseConditionTypeFailedToReconcile, corev1.ConditionTrue, "ReconcileError", err.Error(), now)
	}

	return utils.ChangeTortoiseCondition(t, v1beta3.TortoiseConditionTypeFailedToReconcile, corev1.ConditionFalse, "", "", now)
}

type resourceNameAndContainerName struct {
	rn            corev1.ResourceName
	containerName string
}

// UpdateTortoiseAutoscalingPolicyInStatus updates .status.autoscalingPolicy based on the policy in .spec.autoscalingPolicy,
// and the existing container names in the workload.
func UpdateTortoiseAutoscalingPolicyInStatus(tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, now time.Time) *v1beta3.Tortoise {
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
	// They are the container names which are in the policy but not in the workload - probably removed.
	for _, p := range uselessPolicis.UnsortedList() {
		for i, rp := range tortoise.Status.AutoscalingPolicy {
			if rp.ContainerName == p {
				tortoise.Status.AutoscalingPolicy = append(tortoise.Status.AutoscalingPolicy[:i], tortoise.Status.AutoscalingPolicy[i+1:]...)
				tortoise = utils.RemoveTortoiseResourcePhase(tortoise, rp.ContainerName)
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
		tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p, corev1.ResourceCPU, now, v1beta3.ContainerResourcePhaseGatheringData)
		tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p, corev1.ResourceMemory, now, v1beta3.ContainerResourcePhaseGatheringData)
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
		for i, p := range tortoise.Status.AutoscalingPolicy {
			if hpaManagedResourceAndContainer.Has(resourceNameAndContainerName{corev1.ResourceCPU, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
				if tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] != v1beta3.AutoscalingTypeHorizontal {
					// If HPA has the metrics for this container's CPU, we should set Horizontal.
					tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] = v1beta3.AutoscalingTypeHorizontal
					tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, corev1.ResourceCPU, now, v1beta3.ContainerResourcePhaseGatheringData)
				}
			} else {
				if tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] != v1beta3.AutoscalingTypeVertical {
					// Otherwise, set Vertical.
					tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] = v1beta3.AutoscalingTypeVertical
					tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, corev1.ResourceCPU, now, v1beta3.ContainerResourcePhaseGatheringData)
				}
			}
			if hpaManagedResourceAndContainer.Has(resourceNameAndContainerName{corev1.ResourceMemory, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
				if tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] != v1beta3.AutoscalingTypeHorizontal {
					// If HPA has the metrics for this container's memory, we should set Horizontal.
					tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] = v1beta3.AutoscalingTypeHorizontal
					tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, corev1.ResourceMemory, now, v1beta3.ContainerResourcePhaseGatheringData)
				}
			} else {
				if tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] != v1beta3.AutoscalingTypeVertical {
					// Otherwise, set Vertical.
					tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] = v1beta3.AutoscalingTypeVertical
					tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, corev1.ResourceMemory, now, v1beta3.ContainerResourcePhaseGatheringData)
				}
			}
		}
	}

	// If the container doesn't have resource request, we set the policy to Off because we couldn't make a recommendation.
	for i, p := range tortoise.Status.AutoscalingPolicy {
		if containerWOResourceRequest.Has(resourceNameAndContainerName{corev1.ResourceCPU, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
			tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceCPU] = v1beta3.AutoscalingTypeOff
			tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, corev1.ResourceCPU, now, v1beta3.ContainerResourcePhaseOff)
		}
		if containerWOResourceRequest.Has(resourceNameAndContainerName{corev1.ResourceMemory, tortoise.Status.AutoscalingPolicy[i].ContainerName}) {
			tortoise.Status.AutoscalingPolicy[i].Policy[corev1.ResourceMemory] = v1beta3.AutoscalingTypeOff
			tortoise = utils.ChangeTortoiseContainerResourcePhase(tortoise, p.ContainerName, corev1.ResourceMemory, now, v1beta3.ContainerResourcePhaseOff)
		}
	}

	// sort the autoscaling policy by container name.
	sort.Slice(tortoise.Status.AutoscalingPolicy, func(i, j int) bool {
		return tortoise.Status.AutoscalingPolicy[i].ContainerName < tortoise.Status.AutoscalingPolicy[j].ContainerName
	})
	sort.Slice(tortoise.Status.ContainerResourcePhases, func(i, j int) bool {
		return tortoise.Status.ContainerResourcePhases[i].ContainerName < tortoise.Status.ContainerResourcePhases[j].ContainerName
	})

	return tortoise
}

type containerNameAndResource struct {
	containerName string
	resourceName  corev1.ResourceName
}

// UpdateResourceRequest updates pods' resource requests based on the calculated recommendation.
// Updated ContainerResourceRequests will be used in the next mutating webhook of Pods.
// It updates ContainerResourceRequests in the status of the Tortoise, when ALL the following conditions are met:
//   - UpdateMode is Auto
//   - Any of the recommended resource request is increased,
//     OR, all the recommended resource request is decreased, but it's been a while (1h) after the last update.
func (c *Service) UpdateResourceRequest(ctx context.Context, tortoise *v1beta3.Tortoise, replica int32, now time.Time) (
	*v1beta3.Tortoise,
	error,
) {
	offResources := map[containerNameAndResource]bool{}
	for _, policy := range tortoise.Status.AutoscalingPolicy {
		for rn, p := range policy.Policy {
			if p == v1beta3.AutoscalingTypeOff {
				offResources[containerNameAndResource{containerName: policy.ContainerName, resourceName: rn}] = true
			}
		}
	}

	oldTortoise := tortoise.DeepCopy()

	oldRequestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, r := range tortoise.Status.Conditions.ContainerResourceRequests {
		oldRequestMap[r.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for resourcename, value := range r.Resource {
			oldRequestMap[r.ContainerName][resourcename] = value
		}
	}

	newRequests := make([]v1beta3.ContainerResourceRequests, 0, len(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation))
	for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
		recommendation := r.RecommendedResource.DeepCopy()
		// We only records proposed* metrics here (record applied* metrics later)
		// because we don't want to record applied* metrics when UpdateMode is Off.
		for resourcename, value := range r.RecommendedResource {
			if offResources[containerNameAndResource{containerName: r.ContainerName, resourceName: resourcename}] {
				// ignore
				oldvalue, ok := utils.GetRequestFromTortoise(tortoise, r.ContainerName, resourcename)
				if ok {
					recommendation[resourcename] = oldvalue
				}
				continue
			}

			if resourcename == corev1.ResourceCPU {
				metrics.ProposedCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.MilliValue()))
				if value.IsZero() {
					// This recommendation seems to be invalid. We don't want to set the resource request to 0.
					// Restore the old value.
					oldvalue, ok := utils.GetRequestFromTortoise(tortoise, r.ContainerName, corev1.ResourceCPU)
					if ok {
						log.FromContext(ctx).Error(nil, "The recommended CPU request is 0, which seems to be invalid, restore the old value", "tortoise", tortoise.Name, "namespace", tortoise.Namespace, "container", r.ContainerName, "resource", corev1.ResourceCPU, "oldvalue", oldvalue, "newvalue", value)
						recommendation[corev1.ResourceCPU] = oldvalue
					}
				}
			}
			if resourcename == corev1.ResourceMemory {
				metrics.ProposedMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.Value()))
				if value.IsZero() {
					// This recommendation seems to be invalid. We don't want to set the resource request to 0.
					// Restore the old value.
					oldvalue, ok := utils.GetRequestFromTortoise(tortoise, r.ContainerName, corev1.ResourceMemory)
					if ok {
						log.FromContext(ctx).Error(nil, "The recommended Memory request is 0, which seems to be invalid, restore the old value", "tortoise", tortoise.Name, "namespace", tortoise.Namespace, "container", r.ContainerName, "resource", corev1.ResourceMemory, "oldvalue", oldvalue, "newvalue", value)
						recommendation[corev1.ResourceMemory] = oldvalue
					}
				}
			}
		}
		newRequests = append(newRequests, v1beta3.ContainerResourceRequests{
			ContainerName: r.ContainerName,
			Resource:      recommendation,
		})
	}
	if tortoise.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// nothing to do.
		tortoise = utils.ChangeTortoiseCondition(tortoise,
			v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
			corev1.ConditionFalse,
			"",
			"The recommendation is not provided because it's Off mode",
			now,
		)
		return tortoise, nil
	}

	if tortoise.Status.Conditions.ContainerResourceRequests != nil && reflect.DeepEqual(newRequests, tortoise.Status.Conditions.ContainerResourceRequests) {
		// If the recommendation is not changed at all, we don't need to update VPA and Pods.
		return tortoise, nil
	}

	// The recommendation will be applied to VPA and the deployment will be restarted with the new resources.
	// Update the request recorded in the status, which will be used in the next reconcile loop.
	tortoise.Status.Conditions.ContainerResourceRequests = newRequests

	increased := recommendationIncreaseAnyResource(oldTortoise, tortoise)
	for _, v := range tortoise.Status.Conditions.TortoiseConditions {
		if v.Type == v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated {
			if v.Status == corev1.ConditionTrue {
				// TODO: move the 1h to a config.
				if v.LastTransitionTime.Add(time.Hour).After(now) && !increased {
					// if all the recommended resources is decreased and it's NOT yet been 1h after the last update,
					// we don't want to update the Pod too frequently.
					log.FromContext(ctx).Info("Skip applying vertical recommendation because it's been less than 1h since the last update", "tortoise", tortoise.Name, "namespace", tortoise.Namespace)
					return oldTortoise, nil
				}
			}
		}
	}

	tortoise = utils.ChangeTortoiseCondition(tortoise,
		v1beta3.TortoiseConditionTypeVerticalRecommendationUpdated,
		corev1.ConditionTrue,
		"",
		"The recommendation is provided",
		now,
	)

	c.recorder.Event(tortoise, corev1.EventTypeNormal, event.VerticalRecommendationUpdated, "The vertical recommendation is updated and the Pods should also be updated with new resources soon")

	for _, r := range tortoise.Status.Conditions.ContainerResourceRequests {
		// only record metrics once in every reconcile loop.
		for resourcename, value := range r.Resource {
			oldRequest := oldRequestMap[r.ContainerName][resourcename]
			netChange := float64(oldRequest.MilliValue() - value.MilliValue())
			if resourcename == corev1.ResourceCPU {
				// We don't want to record applied* metric when UpdateMode is Off.
				metrics.AppliedCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.MilliValue()))
				metrics.NetCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(netChange)
			}
			if resourcename == corev1.ResourceMemory {
				metrics.AppliedMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.Value()))
				metrics.NetMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(netChange))
			}
		}
	}

	return tortoise, nil
}

func recommendationIncreaseAnyResource(oldTortoise, newTortoise *v1beta3.Tortoise) bool {
	if newTortoise.Status.Conditions.ContainerResourceRequests == nil {
		// if newVPA doesn't have recommendation, it means we're going to remove the recommendation.
		return true
	}

	for _, new := range newTortoise.Status.Conditions.ContainerResourceRequests {
		found := false
		for _, old := range oldTortoise.Status.Conditions.ContainerResourceRequests {
			if old.ContainerName != new.ContainerName {
				continue
			}

			found = true
			if old.Resource.Cpu().Cmp(*new.Resource.Cpu()) < 0 || old.Resource.Memory().Cmp(*new.Resource.Memory()) < 0 {
				return true
			}
		}
		if !found {
			// if the container is not found in oldTortoise, it means it's the first time to update Tortoise with that container's recommendation.
			return true
		}
	}

	return false
}
