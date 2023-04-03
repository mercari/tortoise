package tortoise

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/sanposhiho/tortoise/api/v1alpha1"
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
}

func New(c client.Client) (*Service, error) {
	timeZone := "Asia/Tokyo"
	jst, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, fmt.Errorf("load location: %w", err)
	}
	return &Service{
		c: c,

		// TODO: make them configurable via flag
		rangeOfMinMaxReplicasRecommendationHour: 1,
		timeZone:                                jst,
	}, nil
}

func (s *Service) InitializeTortoise(tortoise *v1alpha1.Tortoise) *v1alpha1.Tortoise {
	recommendations := []v1alpha1.ReplicasRecommendation{}
	from := 0
	to := s.rangeOfMinMaxReplicasRecommendationHour
	weekDay := time.Sunday
	for {
		recommendations = append(recommendations, v1alpha1.ReplicasRecommendation{
			From:     from,
			To:       to,
			TimeZone: s.timeZone.String(),
			WeekDay:  weekDay,
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
	tortoise.Status.Recommendations.Horizontal.MinReplicas = recommendations
	tortoise.Status.Recommendations.Horizontal.MaxReplicas = recommendations
	tortoise.Status.TortoisePhase = v1alpha1.TortoisePhaseGatheringData
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
				// This case, recommendation is in the acceptable range. We don't update tortoise.
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

func (s *Service) UpdateTortoiseStatus(ctx context.Context, tortoise *v1alpha1.Tortoise) (*v1alpha1.Tortoise, error) {
	return tortoise, s.c.Status().Update(ctx, tortoise)
}
