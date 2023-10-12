package recommender

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta2"
)

type Service struct {
	// configurations
	TTLHourOfMinMaxReplicasRecommendation float64
	maxReplicasFactor                     float64
	minReplicasFactor                     float64

	minimumMinReplicas             int32
	upperTargetResourceUtilization int32
	preferredReplicaNumUpperLimit  int32
	maxResourceSize                corev1.ResourceList
}

func New(
	tTLHoursOfMinMaxReplicasRecommendation int,
	maxReplicasFactor float64,
	minReplicasFactor float64,
	upperTargetResourceUtilization int,
	minimumMinReplicas int,
	preferredReplicaNumUpperLimit int,
	maxCPUPerContainer string,
	maxMemoryPerContainer string,
) *Service {
	return &Service{
		TTLHourOfMinMaxReplicasRecommendation: float64(tTLHoursOfMinMaxReplicasRecommendation),
		maxReplicasFactor:                     maxReplicasFactor,
		minReplicasFactor:                     minReplicasFactor,
		upperTargetResourceUtilization:        int32(upperTargetResourceUtilization),
		minimumMinReplicas:                    int32(minimumMinReplicas),
		preferredReplicaNumUpperLimit:         int32(preferredReplicaNumUpperLimit),
		maxResourceSize: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(maxCPUPerContainer),
			corev1.ResourceMemory: resource.MustParse(maxMemoryPerContainer),
		},
	}
}

func (s *Service) updateVPARecommendation(tortoise *v1beta2.Tortoise, deployment *v1.Deployment, hpa *v2.HorizontalPodAutoscaler) (*v1beta2.Tortoise, error) {
	requestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		requestMap[c.Name] = map[corev1.ResourceName]resource.Quantity{}
		for rn, q := range c.Resources.Requests {
			requestMap[c.Name][rn] = q
		}
	}

	recommendationMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, perContainer := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
		recommendationMap[perContainer.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for k, perResource := range perContainer.Recommendation {
			recommendationMap[perContainer.ContainerName][k] = perResource.Quantity
		}
	}

	newRecommendations := []v1beta2.RecommendedContainerResources{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		recommendation := v1beta2.RecommendedContainerResources{
			ContainerName:       r.ContainerName,
			RecommendedResource: map[corev1.ResourceName]resource.Quantity{},
		}
		for k, p := range r.AutoscalingPolicy {
			reqmap, ok := requestMap[r.ContainerName]
			if !ok {
				klog.ErrorS(nil, fmt.Sprintf("no resource request on the container %s", r.ContainerName))
				continue
			}

			req, ok := reqmap[k]
			if !ok {
				klog.ErrorS(nil, fmt.Sprintf("no %s request on the container %s", k, r.ContainerName))
				continue
			}

			recomMap, ok := recommendationMap[r.ContainerName]
			if !ok {
				return nil, fmt.Errorf("no resource recommendation from VPA for the container %s", r.ContainerName)
			}
			recom, ok := recomMap[k]
			if !ok {
				return nil, fmt.Errorf("no %s recommendation from VPA for the container %s", k, r.ContainerName)
			}
			newSize, err := s.calculateBestNewSize(p, r.ContainerName, recom, k, hpa, deployment, req, r.MinAllocatedResources)
			if err != nil {
				return nil, err
			}

			q := resource.NewMilliQuantity(newSize, req.Format)
			recommendation.RecommendedResource[k] = *q
		}
		newRecommendations = append(newRecommendations, recommendation)
	}

	tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation = newRecommendations

	return tortoise, nil
}

// calculateBestNewSize calculates the best new resource request based on the current replica number and the recommended resource request.
// Even if the autoscaling policy is Horizontal, this function may suggest the vertical scaling, see comments in the function.
func (s *Service) calculateBestNewSize(p v1beta2.AutoscalingType, containerName string, recommendedResourceRequest resource.Quantity, k corev1.ResourceName, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment, resourceRequest resource.Quantity, minAllocatedResources corev1.ResourceList) (int64, error) {
	// When the current replica num is more than or equal to the preferredReplicaNumUpperLimit,
	// make the container size bigger (just multiple by 1.1) so that the replica number will be descreased.
	if deployment.Status.Replicas >= s.preferredReplicaNumUpperLimit {
		// We keep increasing the size until we hit the maxResourceSize.
		newSize := int64(float64(resourceRequest.MilliValue()) * 1.1)
		return s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources), nil
	}

	if deployment.Status.Replicas <= s.minimumMinReplicas || p == v1beta2.AutoscalingTypeVertical {
		// It's the simplest case.
		// The user configures Vertical on this container's resource. This is just vertical scaling.
		newSize := recommendedResourceRequest.MilliValue()
		return s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources), nil
	}

	if deployment.Status.Replicas <= s.minimumMinReplicas || p == v1beta2.AutoscalingTypeVertical {
		// The current replica number is less than or equal to the minimumMinReplicas.
		// The replica number is too small and hits the minReplicas.
		// So, the resource utilization might be super low because HPA cannot scale down further.
		// In this case, we'd like to reduce the resource request as much as possible so that the resource utilization will be higher.
		// And note that we don't increase the resource request even if VPA recommends it.
		// If the resource utilization goes up, HPA does scale up, not VPA.
		newSize := resourceRequest.MilliValue()
		if recommendedResourceRequest.MilliValue() < resourceRequest.MilliValue() {
			// We use the recommended resource request if it's smaller than the current resource request.
			newSize = recommendedResourceRequest.MilliValue()
		}
		return s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources), nil
	}

	if p == v1beta2.AutoscalingTypeHorizontal {
		targetUtilizationValue, err := getHPATargetValue(hpa, containerName, k)
		if err != nil {
			return 0, fmt.Errorf("get the target value from HPA: %w", err)
		}

		upperUtilization := math.Ceil((float64(recommendedResourceRequest.MilliValue()) / float64(resourceRequest.MilliValue())) * 100)
		if targetUtilizationValue > int32(upperUtilization) && len(deployment.Spec.Template.Spec.Containers) >= 2 {
			// upperUtilization is less than targetUtilizationValue, which seems weird in normal cases.
			// In this case, most likely the container size is unbalanced. (= we need multi-container specific optimization)
			// So, for example, when app:istio use the resource in the ratio of 1:5, but the resource request is 1:1,
			// the resource given to istio is always wasted. (since HPA is always kicked by the resource utilization of app)
			//
			// And this case, reducing the resource request of container in this kind of weird situation
			// so that the upper usage will be the target usage.
			newSize := int64(float64(recommendedResourceRequest.MilliValue()) * 100.0 / float64(targetUtilizationValue))
			return s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources), nil
		}
	}

	// Didn't fall into any cases above.
	// Just keep the current resource request.
	return resourceRequest.MilliValue(), nil
}

func (s *Service) justifyNewSizeByMaxMin(newSize int64, k corev1.ResourceName, req resource.Quantity, minAllocatedResources corev1.ResourceList) int64 {
	max := s.maxResourceSize[k]
	min := minAllocatedResources[k]

	if req.MilliValue() > max.MilliValue() {
		return req.MilliValue()
	} else if newSize > max.MilliValue() {
		return max.MilliValue()
	} else if newSize < min.MilliValue() {
		return min.MilliValue()
	}

	return newSize
}

func (s *Service) updateHPARecommendation(ctx context.Context, tortoise *v1beta2.Tortoise, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment, now time.Time) (*v1beta2.Tortoise, error) {
	var err error
	tortoise, err = s.updateHPATargetUtilizationRecommendations(ctx, tortoise, hpa, deployment)
	if err != nil {
		return tortoise, fmt.Errorf("update HPA target utilization recommendations: %w", err)
	}
	tortoise, err = s.updateHPAMinMaxReplicasRecommendations(tortoise, deployment, now)
	if err != nil {
		return tortoise, err
	}

	return tortoise, nil
}

func (s *Service) UpdateRecommendations(ctx context.Context, tortoise *v1beta2.Tortoise, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment, now time.Time) (*v1beta2.Tortoise, error) {
	var err error
	tortoise, err = s.updateHPARecommendation(ctx, tortoise, hpa, deployment, now)
	if err != nil {
		return tortoise, fmt.Errorf("update HPA recommendations: %w", err)
	}
	tortoise, err = s.updateVPARecommendation(tortoise, deployment, hpa)
	if err != nil {
		return tortoise, fmt.Errorf("update VPA recommendations: %w", err)
	}

	return tortoise, nil
}

func (s *Service) updateHPAMinMaxReplicasRecommendations(tortoise *v1beta2.Tortoise, deployment *v1.Deployment, now time.Time) (*v1beta2.Tortoise, error) {
	currentReplicaNum := float64(deployment.Status.Replicas)
	min, err := s.updateMaxMinReplicasRecommendation(int32(math.Ceil(currentReplicaNum*s.minReplicasFactor)), tortoise.Status.Recommendations.Horizontal.MinReplicas, now, s.minimumMinReplicas)
	if err != nil {
		return tortoise, fmt.Errorf("update MinReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MinReplicas = min
	max, err := s.updateMaxMinReplicasRecommendation(int32(math.Ceil(currentReplicaNum*s.maxReplicasFactor)), tortoise.Status.Recommendations.Horizontal.MaxReplicas, now, int32(float64(s.minimumMinReplicas)*s.maxReplicasFactor/s.minReplicasFactor))
	if err != nil {
		return tortoise, fmt.Errorf("update MaxReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MaxReplicas = max

	return tortoise, nil
}

// updateMaxMinReplicasRecommendation replaces value if the value is higher than the current value.
func (s *Service) updateMaxMinReplicasRecommendation(value int32, recommendations []v1beta2.ReplicasRecommendation, now time.Time, minimum int32) ([]v1beta2.ReplicasRecommendation, error) {
	// find the corresponding recommendations.
	index := -1
	for i, r := range recommendations {
		tz, err := time.LoadLocation(r.TimeZone)
		if err == nil {
			// if the timezone is invalid, just ignore it.
			now = now.In(tz)
		}
		if now.Hour() < r.To && now.Hour() >= r.From && (r.WeekDay == nil || now.Weekday().String() == *r.WeekDay) {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, errors.New("no recommendation slot")
	}
	if value <= minimum {
		value = minimum
	}
	if now.Sub(recommendations[index].UpdatedAt.Time).Hours() < s.TTLHourOfMinMaxReplicasRecommendation && value < recommendations[index].Value {
		return recommendations, nil
	}

	recommendations[index].UpdatedAt = metav1.NewTime(now)
	recommendations[index].Value = value
	return recommendations, nil
}

func (s *Service) updateHPATargetUtilizationRecommendations(ctx context.Context, tortoise *v1beta2.Tortoise, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment) (*v1beta2.Tortoise, error) {
	logger := log.FromContext(ctx)
	requestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		requestMap[c.Name] = map[corev1.ResourceName]resource.Quantity{}
		for rn, q := range c.Resources.Requests {
			requestMap[c.Name][rn] = q
		}
	}

	recommendationMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, perContainer := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
		recommendationMap[perContainer.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for k, perResource := range perContainer.MaxRecommendation {
			recommendationMap[perContainer.ContainerName][k] = perResource.Quantity
		}
	}

	newHPATargetUtilizationRecommendationPerContainer := []v1beta2.HPATargetUtilizationRecommendationPerContainer{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		targetMap := map[corev1.ResourceName]int32{}
		reqmap, ok := requestMap[r.ContainerName]
		if !ok {
			klog.ErrorS(nil, fmt.Sprintf("no resource request on the container %s", r.ContainerName))
			continue
		}
		for k, p := range r.AutoscalingPolicy {
			if p != v1beta2.AutoscalingTypeHorizontal {
				// nothing to do.
				continue
			}

			req, ok := reqmap[k]
			if !ok {
				klog.ErrorS(nil, fmt.Sprintf("no %s request on the container %s", k, r.ContainerName))
				continue
			}

			targetValue, err := getHPATargetValue(hpa, r.ContainerName, k)
			if err != nil {
				return tortoise, fmt.Errorf("try to find the metric for the conainter which is configured to be scale by Horizontal: %w", err)
			}

			recomMap, ok := recommendationMap[r.ContainerName]
			if !ok {
				return tortoise, fmt.Errorf("no resource recommendation from VPA for the container %s", r.ContainerName)
			}
			recom, ok := recomMap[k]
			if !ok {
				return tortoise, fmt.Errorf("no %s recommendation from VPA for the container %s", k, r.ContainerName)
			}

			upperUsage := math.Ceil((float64(recom.MilliValue()) / float64(req.MilliValue())) * 100)
			if targetValue > int32(upperUsage) && len(requestMap) >= 2 {
				// upperUsage is less than targetValue.
				// This case, most likely the container size is unbalanced. (one resource is very bigger than its consumption)
				// https://github.com/mercari/tortoise/issues/24
				// And this case, rather than changing the target value, we'd like to change the container size.
				targetMap[k] = targetValue
			} else {
				targetMap[k] = updateRecommendedContainerBasedMetric(int32(upperUsage), targetValue)
				if targetMap[k] > s.upperTargetResourceUtilization {
					targetMap[k] = s.upperTargetResourceUtilization
				}
			}
			logger.Info("HPA target utilization recommendation is created", "current target utilization", targetValue, "recommended target utilization", targetMap[k], "upper usage", upperUsage, "container name", r.ContainerName, "resource name", k)
		}
		newHPATargetUtilizationRecommendationPerContainer = append(newHPATargetUtilizationRecommendationPerContainer, v1beta2.HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     r.ContainerName,
			TargetUtilization: targetMap,
		})
	}

	tortoise.Status.Recommendations.Horizontal.TargetUtilizations = newHPATargetUtilizationRecommendationPerContainer

	return tortoise, nil
}

// getHPATargetValue gets the target value of the HPA.
// It looks for the corresponding metric (ContainerResource) and gets the target value.
func getHPATargetValue(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName) (int32, error) {
	for _, m := range hpa.Spec.Metrics {
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

		return *m.ContainerResource.Target.AverageUtilization, nil
	}

	return 0, fmt.Errorf("the metric for the container isn't found in the hpa: %s. (resource name: %s, container name: %s)", client.ObjectKeyFromObject(hpa).String(), k, containerName)
}

func updateRecommendedContainerBasedMetric(upperUsage, currentTarget int32) int32 {
	additionalResource := upperUsage - currentTarget
	return 100 - additionalResource
}
