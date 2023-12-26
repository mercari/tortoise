package recommender

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/event"
	hpaservice "github.com/mercari/tortoise/pkg/hpa"
)

type Service struct {
	// configurations
	TTLHourOfMinMaxReplicasRecommendation float64
	maxReplicasFactor                     float64
	minReplicasFactor                     float64

	eventRecorder                  record.EventRecorder
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
	eventRecorder record.EventRecorder,
) *Service {
	return &Service{
		eventRecorder:                         eventRecorder,
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

func (s *Service) updateVPARecommendation(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32) (*v1beta3.Tortoise, error) {
	logger := log.FromContext(ctx)
	requestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	// This ContainerResourceRecommendationkshould be the current resource requests.
	for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
		requestMap[r.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for resourcename, value := range r.RecommendedResource {
			requestMap[r.ContainerName][resourcename] = value
		}
	}

	recommendationMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, perContainer := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
		recommendationMap[perContainer.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for k, perResource := range perContainer.Recommendation {
			recommendationMap[perContainer.ContainerName][k] = perResource.Quantity
		}
	}

	// containerName → MinAllocatedResources
	minAllocatedResourcesMap := map[string]v1.ResourceList{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		minAllocatedResourcesMap[r.ContainerName] = r.MinAllocatedResources
	}

	newRecommendations := []v1beta3.RecommendedContainerResources{}
	for _, r := range tortoise.Status.AutoscalingPolicy {
		recommendation := v1beta3.RecommendedContainerResources{
			ContainerName:       r.ContainerName,
			RecommendedResource: map[corev1.ResourceName]resource.Quantity{},
		}
		for k, p := range r.Policy {
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
			newSize, reason, err := s.calculateBestNewSize(ctx, p, r.ContainerName, recom, k, hpa, replicaNum, req, minAllocatedResourcesMap[r.ContainerName], len(requestMap) > 1)
			if err != nil {
				return nil, err
			}

			if newSize != req.MilliValue() {
				s.eventRecorder.Event(tortoise, corev1.EventTypeNormal, event.RecommendationUpdated, fmt.Sprintf("The recommendation of %v request (%v) in Tortoise status is updated. Reason: %v", k, r.ContainerName, reason))
			} else {
				logger.V(4).Info("The recommendation of the container is not updated", "container name", r.ContainerName, "resource name", k, "reason", reason)
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
func (s *Service) calculateBestNewSize(ctx context.Context, p v1beta3.AutoscalingType, containerName string, recommendedResourceRequest resource.Quantity, k corev1.ResourceName, hpa *v2.HorizontalPodAutoscaler, replicaNum int32, resourceRequest resource.Quantity, minAllocatedResources corev1.ResourceList, isMultipleContainersPod bool) (int64, string, error) {
	// When the current replica num is more than or equal to the preferredReplicaNumUpperLimit,
	// make the container size bigger (just multiple by 1.1) so that the replica number will be descreased.
	if replicaNum >= s.preferredReplicaNumUpperLimit {
		// We keep increasing the size until we hit the maxResourceSize.
		newSize := int64(float64(resourceRequest.MilliValue()) * 1.1)
		jastifiedNewSize := s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources)
		return jastifiedNewSize, fmt.Sprintf("the current number of replicas is bigger than the preferred max replica number in this cluster (%v), so make %v request (%s) bigger (%v → %v)", s.preferredReplicaNumUpperLimit, k, containerName, resourceRequest.MilliValue(), jastifiedNewSize), nil
	}

	if replicaNum <= s.minimumMinReplicas || p == v1beta3.AutoscalingTypeVertical {
		// It's the simplest case.
		// The user configures Vertical on this container's resource. This is just vertical scaling.
		newSize := recommendedResourceRequest.MilliValue()
		jastified := s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources)
		return jastified, fmt.Sprintf("change %v request (%v) (%v → %v) based on VPA suggestion", k, containerName, resourceRequest.MilliValue(), jastified), nil
	}

	if replicaNum <= s.minimumMinReplicas || p == v1beta3.AutoscalingTypeVertical {
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
		jastified := s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources)

		return jastified, fmt.Sprintf("the current number of replicas is equal or smaller than the minimum min replica number in this cluster (%v), so make %v request (%v) smaller (%v → %v) based on VPA suggestion", s.minimumMinReplicas, k, containerName, resourceRequest.MilliValue(), jastified), nil
	}

	if p == v1beta3.AutoscalingTypeHorizontal {
		targetUtilizationValue, err := hpaservice.GetHPATargetValue(ctx, hpa, containerName, k)
		if err != nil {
			return 0, "", fmt.Errorf("get the target value from HPA: %w", err)
		}

		upperUtilization := math.Ceil((float64(recommendedResourceRequest.MilliValue()) / float64(resourceRequest.MilliValue())) * 100)
		if targetUtilizationValue > int32(upperUtilization) && isMultipleContainersPod {
			// upperUtilization is less than targetUtilizationValue, which seems weird in normal cases.
			// In this case, most likely the container size is unbalanced. (= we need multi-container specific optimization)
			// So, for example, when app:istio use the resource in the ratio of 1:5, but the resource request is 1:1,
			// the resource given to istio is always wasted. (since HPA is always kicked by the resource utilization of app)
			//
			// And this case, reducing the resource request of container in this kind of weird situation
			// so that the upper usage will be the target usage.
			newSize := int64(float64(recommendedResourceRequest.MilliValue()) * 100.0 / float64(targetUtilizationValue))
			jastified := s.justifyNewSizeByMaxMin(newSize, k, resourceRequest, minAllocatedResources)
			return jastified, fmt.Sprintf("the current resource utilization (%v) is too small and it's due to unbalanced container size, so make %v request (%v) smaller (%v → %v) based on VPA's recommendation and HPA target utilization %v%%", int(upperUtilization), k, containerName, resourceRequest.MilliValue(), jastified, targetUtilizationValue), nil
		}
	}

	// Didn't fall into any cases above.
	// Just keep the current resource request.
	return resourceRequest.MilliValue(), "", nil
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

func (s *Service) updateHPARecommendation(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32, now time.Time) (*v1beta3.Tortoise, error) {
	var err error
	tortoise, err = s.updateHPATargetUtilizationRecommendations(ctx, tortoise, hpa)
	if err != nil {
		return tortoise, fmt.Errorf("update HPA target utilization recommendations: %w", err)
	}
	tortoise, err = s.updateHPAMinMaxReplicasRecommendations(tortoise, replicaNum, now)
	if err != nil {
		return tortoise, err
	}

	return tortoise, nil
}

func (s *Service) UpdateRecommendations(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32, now time.Time) (*v1beta3.Tortoise, error) {
	var err error
	tortoise, err = s.updateHPARecommendation(ctx, tortoise, hpa, replicaNum, now)
	if err != nil {
		return tortoise, fmt.Errorf("update HPA recommendations: %w", err)
	}
	tortoise, err = s.updateVPARecommendation(ctx, tortoise, hpa, replicaNum)
	if err != nil {
		return tortoise, fmt.Errorf("update VPA recommendations: %w", err)
	}

	return tortoise, nil
}

func (s *Service) updateHPAMinMaxReplicasRecommendations(tortoise *v1beta3.Tortoise, replicaNum int32, now time.Time) (*v1beta3.Tortoise, error) {
	currentReplicaNum := float64(replicaNum)
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
func (s *Service) updateMaxMinReplicasRecommendation(value int32, recommendations []v1beta3.ReplicasRecommendation, now time.Time, minimum int32) ([]v1beta3.ReplicasRecommendation, error) {
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

func (s *Service) updateHPATargetUtilizationRecommendations(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler) (*v1beta3.Tortoise, error) {
	logger := log.FromContext(ctx)

	requestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	// This ContainerResourceRecommendationkshould be the current resource requests.
	for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
		requestMap[r.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for resourcename, value := range r.RecommendedResource {
			requestMap[r.ContainerName][resourcename] = value
		}
	}

	recommendationMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, perContainer := range tortoise.Status.Conditions.ContainerRecommendationFromVPA {
		recommendationMap[perContainer.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for k, perResource := range perContainer.MaxRecommendation {
			recommendationMap[perContainer.ContainerName][k] = perResource.Quantity
		}
	}

	newHPATargetUtilizationRecommendationPerContainer := []v1beta3.HPATargetUtilizationRecommendationPerContainer{}
	for _, r := range tortoise.Status.AutoscalingPolicy {
		recommendedTargetUtilization := map[corev1.ResourceName]int32{}
		reqmap, ok := requestMap[r.ContainerName]
		if !ok {
			klog.ErrorS(nil, fmt.Sprintf("no resource request on the container %s", r.ContainerName))
			continue
		}
		for k, p := range r.Policy {
			if p != v1beta3.AutoscalingTypeHorizontal {
				// nothing to do.
				continue
			}

			req, ok := reqmap[k]
			if !ok {
				klog.ErrorS(nil, fmt.Sprintf("no %s request on the container %s", k, r.ContainerName))
				continue
			}

			currentTargetValue, err := hpaservice.GetHPATargetValue(ctx, hpa, r.ContainerName, k)
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
			if currentTargetValue > int32(upperUsage) && len(requestMap) >= 2 {
				// upperUsage is less than targetValue.
				// This case, most likely the container size is unbalanced. (one resource is very bigger than its consumption)
				// https://github.com/mercari/tortoise/issues/24
				// And this case, rather than changing the target value, we'd like to change the container size.
				recommendedTargetUtilization[k] = currentTargetValue // no change
			} else {
				recommendedTargetUtilization[k] = updateRecommendedContainerBasedMetric(int32(upperUsage), currentTargetValue)
				if recommendedTargetUtilization[k] > s.upperTargetResourceUtilization {
					recommendedTargetUtilization[k] = s.upperTargetResourceUtilization
				}
			}

			if currentTargetValue != recommendedTargetUtilization[k] {
				s.eventRecorder.Event(tortoise, corev1.EventTypeNormal, event.RecommendationUpdated, fmt.Sprintf("The recommendation of HPA %v target utilization (%v) in Tortoise status is updated (%v%% → %v%%)", k, r.ContainerName, currentTargetValue, recommendedTargetUtilization[k]))
			} else {
				logger.V(4).Info("The recommendation of the container is not updated", "container name", r.ContainerName, "resource name", k, "reason", fmt.Sprintf("HPA target utilization %v%% → %v%%", currentTargetValue, recommendedTargetUtilization[k]))
			}

			logger.Info("HPA target utilization recommendation is created", "current target utilization", currentTargetValue, "recommended target utilization", recommendedTargetUtilization[k], "upper usage", upperUsage, "container name", r.ContainerName, "resource name", k)
		}
		newHPATargetUtilizationRecommendationPerContainer = append(newHPATargetUtilizationRecommendationPerContainer, v1beta3.HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     r.ContainerName,
			TargetUtilization: recommendedTargetUtilization,
		})
	}

	tortoise.Status.Recommendations.Horizontal.TargetUtilizations = newHPATargetUtilizationRecommendationPerContainer

	return tortoise, nil
}

func updateRecommendedContainerBasedMetric(upperUsage, currentTarget int32) int32 {
	additionalResource := upperUsage - currentTarget
	return 100 - additionalResource
}
