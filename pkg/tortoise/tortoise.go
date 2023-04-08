package tortoise

import (
	"context"
	"fmt"
	"github.com/mercari/tortoise/pkg/utils"
	appv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/mercari/tortoise/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	c                                       client.Client
	rangeOfMinMaxReplicasRecommendationHour int
	timeZone                                *time.Location
	tortoiseUpdateInterval                  time.Duration

	mu                     sync.RWMutex
	lastTimeUpdateTortoise map[client.ObjectKey]time.Time
}

func New(c client.Client, rangeOfMinMaxReplicasRecommendationHour int, timeZone string, tortoiseUpdateInterval time.Duration) (*Service, error) {
	jst, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, fmt.Errorf("load location: %w", err)
	}
	return &Service{
		c: c,

		rangeOfMinMaxReplicasRecommendationHour: rangeOfMinMaxReplicasRecommendationHour,
		timeZone:                                jst,
		tortoiseUpdateInterval:                  tortoiseUpdateInterval,
		lastTimeUpdateTortoise:                  map[client.ObjectKey]time.Time{},
	}, nil
}

func (s *Service) ShouldReconcileTortoiseNow(tortoise *v1alpha1.Tortoise, now time.Time) (bool, time.Duration) {
	if tortoise.Spec.UpdateMode == v1alpha1.UpdateModeEmergency {
		// If Emergency, it should be updated ASAP.
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

func (s *Service) UpdateTortoisePhase(tortoise *v1alpha1.Tortoise, dm *appv1.Deployment) *v1alpha1.Tortoise {
	switch tortoise.Status.TortoisePhase {
	case "":
		tortoise = s.initializeTortoise(tortoise, dm)
	case v1alpha1.TortoisePhaseInitializing:
		// TODO: check initializing finished. (if VPA/HPA are working well etc)
		tortoise.Status.TortoisePhase = v1alpha1.TortoisePhaseGatheringData
	case v1alpha1.TortoisePhaseGatheringData:
		tortoise = s.checkIfTortoiseFinishedGatheringData(tortoise)
	}

	if tortoise.Spec.UpdateMode == v1alpha1.UpdateModeEmergency {
		tortoise.Status.TortoisePhase = v1alpha1.TortoisePhaseEmergency
	}
	return tortoise
}

func (s *Service) checkIfTortoiseFinishedGatheringData(tortoise *v1alpha1.Tortoise) *v1alpha1.Tortoise {
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

	tortoise.Status.TortoisePhase = v1alpha1.TortoisePhaseWorking
	return tortoise
}

func (s *Service) initializeTortoise(tortoise *v1alpha1.Tortoise, dm *appv1.Deployment) *v1alpha1.Tortoise {
	recommendations := []v1alpha1.ReplicasRecommendation{}
	from := 0
	to := s.rangeOfMinMaxReplicasRecommendationHour
	weekDay := time.Sunday
	for {
		recommendations = append(recommendations, v1alpha1.ReplicasRecommendation{
			From:     from,
			To:       to,
			TimeZone: s.timeZone.String(),
			WeekDay:  weekDay.String(),
		})
		if to == 24 {
			if weekDay == time.Saturday {
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
	if tortoise.Status.Recommendations.Horizontal == nil {
		tortoise.Status.Recommendations.Horizontal = &v1alpha1.HorizontalRecommendations{}
	}
	tortoise.Status.Recommendations.Horizontal.MinReplicas = recommendations
	tortoise.Status.Recommendations.Horizontal.MaxReplicas = recommendations
	tortoise.Status.TortoisePhase = v1alpha1.TortoisePhaseInitializing

	tortoise.Status.Conditions.ContainerRecommendationFromVPA = make([]v1alpha1.ContainerRecommendationFromVPA, len(dm.Spec.Template.Spec.Containers))
	for i, c := range dm.Spec.Template.Spec.Containers {
		tortoise.Status.Conditions.ContainerRecommendationFromVPA[i] = v1alpha1.ContainerRecommendationFromVPA{
			ContainerName: c.Name,
			Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
			MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
				corev1.ResourceCPU:    {},
				corev1.ResourceMemory: {},
			},
		}
	}
	tortoise.Status.Targets.Deployment = dm.Name

	return tortoise.DeepCopy()
}

func (s *Service) UpdateUpperRecommendation(tortoise *v1alpha1.Tortoise, vpa *v1.VerticalPodAutoscaler) *v1alpha1.Tortoise {
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

			rq := v1alpha1.ResourceQuantity{
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

func (s *Service) GetTortoise(ctx context.Context, namespacedName types.NamespacedName) (*v1alpha1.Tortoise, error) {
	t := &v1alpha1.Tortoise{}
	if err := s.c.Get(ctx, namespacedName, t); err != nil {
		return nil, fmt.Errorf("failed to get tortoise: %w", err)
	}
	return t, nil
}

func (s *Service) UpdateTortoiseStatus(ctx context.Context, originalTortoise *v1alpha1.Tortoise, now time.Time) (*v1alpha1.Tortoise, error) {
	logger := log.FromContext(ctx)
	logger.V(4).Info("update tortoise status", "tortoise", klog.KObj(originalTortoise))
	updateFn := func() (bool, error) {
		tortoise := &v1alpha1.Tortoise{}
		err := s.c.Get(ctx, client.ObjectKeyFromObject(originalTortoise), tortoise)
		if err != nil {
			return true, err
		}
		// It should be OK to overwrite the status, because the controller is the only person to update it.
		tortoise.Status = originalTortoise.Status

		err = s.c.Status().Update(ctx, tortoise)
		if err != nil {
			if apierrors.IsConflict(err) {
				return false, nil
			}
			return true, err
		}
		return true, nil
	}

	err := utils.RetryWithExponentialBackOff(updateFn)
	if err != nil {
		return originalTortoise, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastTimeUpdateTortoise[client.ObjectKeyFromObject(originalTortoise)] = now
	return originalTortoise, nil
}
