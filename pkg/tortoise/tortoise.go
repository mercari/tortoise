package tortoise

import (
	"github.com/sanposhiho/tortoise/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	c client.Client
}

func New(c client.Client) Service {
	return Service{c: c}
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
