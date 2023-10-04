package tortoise

import (
	"context"
	"fmt"
	"sync"
	"time"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta1"
)

const tortoiseFinalizer = "tortoise.autoscaling.mercari.com/finalizer"

type Service struct {
	c        client.Client
	recorder record.EventRecorder

	rangeOfMinMaxReplicasRecommendationHour int
	timeZone                                *time.Location
	tortoiseUpdateInterval                  time.Duration
	// If "daily", tortoise will consider all workload behaves very similarly every day.
	// If your workload may behave differently on, for example, weekdays and weekends, set this to "".
	minMaxReplicasRoutine string

	mu sync.RWMutex
	// TODO: Instead of here, we should store the last time of each tortoise in the status of the tortoise.
	lastTimeUpdateTortoise map[client.ObjectKey]time.Time
}

func New(c client.Client, recorder record.EventRecorder, rangeOfMinMaxReplicasRecommendationHour int, timeZone string, tortoiseUpdateInterval time.Duration, minMaxReplicasRoutine string) (*Service, error) {
	jst, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, fmt.Errorf("load location: %w", err)
	}

	return &Service{
		c: c,

		recorder:                                recorder,
		rangeOfMinMaxReplicasRecommendationHour: rangeOfMinMaxReplicasRecommendationHour,
		minMaxReplicasRoutine:                   minMaxReplicasRoutine,
		timeZone:                                jst,
		tortoiseUpdateInterval:                  tortoiseUpdateInterval,
		lastTimeUpdateTortoise:                  map[client.ObjectKey]time.Time{},
	}, nil
}

func (s *Service) ShouldReconcileTortoiseNow(tortoise *v1beta1.Tortoise, now time.Time) (bool, time.Duration) {
	if tortoise.Spec.UpdateMode == v1beta1.UpdateModeEmergency && tortoise.Status.TortoisePhase != v1beta1.TortoisePhaseEmergency {
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

func (s *Service) UpdateTortoisePhase(tortoise *v1beta1.Tortoise, dm *appv1.Deployment) *v1beta1.Tortoise {
	switch tortoise.Status.TortoisePhase {
	case "":
		tortoise = s.initializeTortoise(tortoise, dm)
		r := "1 week"
		if s.minMaxReplicasRoutine == "daily" {
			r = "1 day"
		}
		s.recorder.Event(tortoise, corev1.EventTypeNormal, "Initialized", fmt.Sprintf("Tortoise is initialized and starts to gather data to make recommendations. It will take %s to finish gathering data and then tortoise starts to work actually", r))

	case v1beta1.TortoisePhaseInitializing:
		// change it to GatheringData anyway. Later the controller may change it back to initialize if VPA isn't ready.
		tortoise.Status.TortoisePhase = v1beta1.TortoisePhaseGatheringData
	case v1beta1.TortoisePhaseGatheringData:
		tortoise = s.checkIfTortoiseFinishedGatheringData(tortoise)
		if tortoise.Status.TortoisePhase == v1beta1.TortoisePhaseWorking {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, "Working", "Tortoise finishes gathering data and it starts to work on autoscaling")
		}
	case v1beta1.TortoisePhaseEmergency:
		if tortoise.Spec.UpdateMode != v1beta1.UpdateModeEmergency {
			// Emergency mode is turned off.
			s.recorder.Event(tortoise, corev1.EventTypeNormal, "Working", "Emergency mode is turned off. Tortoise starts to work on autoscaling normally")
			tortoise.Status.TortoisePhase = v1beta1.TortoisePhaseEmergency
		}
	}

	if tortoise.Spec.UpdateMode == v1beta1.UpdateModeEmergency {
		if tortoise.Status.TortoisePhase != v1beta1.TortoisePhaseEmergency {
			s.recorder.Event(tortoise, corev1.EventTypeNormal, "Emergency", "Tortoise is in Emergency mode")
			tortoise.Status.TortoisePhase = v1beta1.TortoisePhaseEmergency
		}
	}

	return tortoise
}

func (s *Service) checkIfTortoiseFinishedGatheringData(tortoise *v1beta1.Tortoise) *v1beta1.Tortoise {
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

	tortoise.Status.TortoisePhase = v1beta1.TortoisePhaseWorking
	return tortoise
}

func (s *Service) initializeMinMaxReplicas(tortoise *v1beta1.Tortoise) *v1beta1.Tortoise {
	recommendations := []v1beta1.ReplicasRecommendation{}
	from := 0
	to := s.rangeOfMinMaxReplicasRecommendationHour
	weekDay := time.Sunday
	for {
		if s.minMaxReplicasRoutine == "daily" {
			recommendations = append(recommendations, v1beta1.ReplicasRecommendation{
				From:     from,
				To:       to,
				TimeZone: s.timeZone.String(),
			})
		} else if s.minMaxReplicasRoutine == "weekly" {
			recommendations = append(recommendations, v1beta1.ReplicasRecommendation{
				From:     from,
				To:       to,
				TimeZone: s.timeZone.String(),
				WeekDay:  pointer.String(weekDay.String()),
			})
		}

		if to == 24 {
			if weekDay == time.Saturday || s.minMaxReplicasRoutine == "daily" {
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

func (s *Service) initializeTortoise(tortoise *v1beta1.Tortoise, dm *appv1.Deployment) *v1beta1.Tortoise {
	tortoise = s.initializeMinMaxReplicas(tortoise)
	tortoise.Status.TortoisePhase = v1beta1.TortoisePhaseInitializing

	tortoise.Status.Conditions.ContainerRecommendationFromVPA = make([]v1beta1.ContainerRecommendationFromVPA, len(dm.Spec.Template.Spec.Containers))
	for i, c := range dm.Spec.Template.Spec.Containers {
		tortoise.Status.Conditions.ContainerRecommendationFromVPA[i] = v1beta1.ContainerRecommendationFromVPA{
			ContainerName: c.Name,
			Recommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
			MaxRecommendation: map[corev1.ResourceName]v1beta1.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
		}
	}
	tortoise.Status.Targets.Deployment = dm.Name

	return tortoise.DeepCopy()
}

func (s *Service) UpdateUpperRecommendation(tortoise *v1beta1.Tortoise, vpa *v1.VerticalPodAutoscaler) *v1beta1.Tortoise {
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
			currentUpper := upperMap[r.ContainerName][rn]
			currentTarget := targetMap[r.ContainerName][rn]
			recommendation := max.Quantity

			rq := v1beta1.ResourceQuantity{
				Quantity:  currentTarget,
				UpdatedAt: metav1.Now(),
			}

			tortoise.Status.Conditions.ContainerRecommendationFromVPA[k].Recommendation[rn] = rq
			if recommendation.Cmp(currentTarget) > 0 && recommendation.Cmp(currentUpper) < 0 {
				// This case, recommendation is in the acceptable range. We don't update maxRecommendation.
				continue
			}

			tortoise.Status.Conditions.ContainerRecommendationFromVPA[k].MaxRecommendation[rn] = rq
		}
	}
	return tortoise
}

func (s *Service) GetTortoise(ctx context.Context, namespacedName types.NamespacedName) (*v1beta1.Tortoise, error) {
	t := &v1beta1.Tortoise{}
	if err := s.c.Get(ctx, namespacedName, t); err != nil {
		return nil, fmt.Errorf("failed to get tortoise: %w", err)
	}
	return t, nil
}

func (s *Service) AddFinalizer(ctx context.Context, tortoise *v1beta1.Tortoise) error {
	if controllerutil.ContainsFinalizer(tortoise, tortoiseFinalizer) {
		return nil
	}

	updateFn := func() error {
		retTortoise := &v1beta1.Tortoise{}
		err := s.c.Get(ctx, client.ObjectKeyFromObject(tortoise), retTortoise)
		if err != nil {
			return err
		}
		controllerutil.AddFinalizer(retTortoise, tortoiseFinalizer)
		return s.c.Update(ctx, retTortoise)
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, updateFn)
	if err != nil {
		return fmt.Errorf("failed to add finalizer: %w", err)
	}

	return nil
}

func (s *Service) RemoveFinalizer(ctx context.Context, tortoise *v1beta1.Tortoise) error {
	if !controllerutil.ContainsFinalizer(tortoise, tortoiseFinalizer) {
		return nil
	}

	updateFn := func() error {
		retTortoise := &v1beta1.Tortoise{}
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

func (s *Service) UpdateTortoiseStatus(ctx context.Context, originalTortoise *v1beta1.Tortoise, now time.Time) (*v1beta1.Tortoise, error) {
	logger := log.FromContext(ctx)
	logger.V(4).Info("update tortoise status", "tortoise", klog.KObj(originalTortoise))
	retTortoise := &v1beta1.Tortoise{}
	updateFn := func() error {
		retTortoise = &v1beta1.Tortoise{}
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

	s.updateLastTimeUpdateTortoise(originalTortoise, now)

	return originalTortoise, nil
}

func (s *Service) updateLastTimeUpdateTortoise(tortoise *v1beta1.Tortoise, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastTimeUpdateTortoise[client.ObjectKeyFromObject(tortoise)] = now
}
