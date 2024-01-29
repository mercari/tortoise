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
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/event"
	hpaservice "github.com/mercari/tortoise/pkg/hpa"
)

type Service struct {
	// configurations
	maxReplicasFactor float64
	minReplicasFactor float64

	eventRecorder                    record.EventRecorder
	minimumMinReplicas               int32
	maximumTargetResourceUtilization int32
	minimumTargetResourceUtilization int32
	preferredReplicaNumUpperLimit    int32
	maxResourceSize                  corev1.ResourceList
	minResourceSize                  corev1.ResourceList
	maximumMaxReplica                int32
}

func New(
	maxReplicasFactor float64,
	minReplicasFactor float64,
	maximumTargetResourceUtilization int,
	minimumTargetResourceUtilization int,
	minimumMinReplicas int,
	preferredReplicaNumUpperLimit int,
	minCPUPerContainer string,
	minMemoryPerContainer string,
	maxCPUPerContainer string,
	maxMemoryPerContainer string,
	maximumMaxReplica int32,
	eventRecorder record.EventRecorder,
) *Service {
	return &Service{
		eventRecorder:                    eventRecorder,
		maxReplicasFactor:                maxReplicasFactor,
		minReplicasFactor:                minReplicasFactor,
		maximumTargetResourceUtilization: int32(maximumTargetResourceUtilization),
		minimumTargetResourceUtilization: int32(minimumTargetResourceUtilization),
		minimumMinReplicas:               int32(minimumMinReplicas),
		preferredReplicaNumUpperLimit:    int32(preferredReplicaNumUpperLimit),
		maxResourceSize: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(maxCPUPerContainer),
			corev1.ResourceMemory: resource.MustParse(maxMemoryPerContainer),
		},
		minResourceSize: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(minCPUPerContainer),
			corev1.ResourceMemory: resource.MustParse(minMemoryPerContainer),
		},
		maximumMaxReplica: maximumMaxReplica,
	}
}

func (s *Service) updateVPARecommendation(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32) (*v1beta3.Tortoise, error) {
	logger := log.FromContext(ctx)
	requestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, r := range tortoise.Status.Conditions.ContainerResourceRequests {
		requestMap[r.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for resourcename, value := range r.Resource {
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
				if p != v1beta3.AutoscalingTypeOff {
					logger.Error(nil, fmt.Sprintf("no resource request on the container %s, but the resource %s of this container has %s autoscaling policy", r.ContainerName, k, p))
				}
				continue
			}

			req, ok := reqmap[k]
			if !ok {
				if p != v1beta3.AutoscalingTypeOff {
					logger.Error(nil, fmt.Sprintf("no %s request on the container %s, but this resource has %s autoscaling policy", k, r.ContainerName, p))
				}
				continue
			}

			recomMap, ok := recommendationMap[r.ContainerName]
			if !ok {
				return tortoise, fmt.Errorf("no resource recommendation from VPA for the container %s", r.ContainerName)
			}
			recom, ok := recomMap[k]
			if !ok {
				return tortoise, fmt.Errorf("no %s recommendation from VPA for the container %s", k, r.ContainerName)
			}
			newSize, reason, err := s.calculateBestNewSize(ctx, p, r.ContainerName, recom, k, hpa, replicaNum, req, minAllocatedResourcesMap[r.ContainerName], len(requestMap) > 1)
			if err != nil {
				return tortoise, err
			}

			if newSize != req.MilliValue() {
				s.eventRecorder.Event(tortoise, corev1.EventTypeNormal, event.RecommendationUpdated, fmt.Sprintf("The recommendation of %v request (%v) in Tortoise status is updated. Reason: %v", k, r.ContainerName, reason))
			} else {
				logger.Info("The recommendation of the container is not updated", "container name", r.ContainerName, "resource name", k, "reason", reason)
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
	if p == v1beta3.AutoscalingTypeOff {
		// Just keep the current resource request.
		return resourceRequest.MilliValue(), "", nil
	}

	if p == v1beta3.AutoscalingTypeVertical {
		// It's the simplest case.
		// The user configures Vertical on this container's resource. This is just vertical scaling.
		// We always follow the recommendation from VPA.
		newSize := recommendedResourceRequest.MilliValue()
		jastified := s.justifyNewSizeByMaxMin(newSize, k, minAllocatedResources)
		return jastified, fmt.Sprintf("change %v request (%v) (%v → %v) based on VPA suggestion", k, containerName, resourceRequest.MilliValue(), jastified), nil
	}

	// p == v1beta3.AutoscalingTypeHorizontal

	// When the current replica num is more than or equal to the preferredReplicaNumUpperLimit,
	// make the container size bigger (just multiple by 1.1) so that the replica number will be descreased.
	//
	// Here also covers the scenario where the current replica num hits MaximumMaxReplicas.
	if replicaNum >= s.preferredReplicaNumUpperLimit {
		// We keep increasing the size until we hit the maxResourceSize.
		newSize := int64(float64(resourceRequest.MilliValue()) * 1.1)
		jastifiedNewSize := s.justifyNewSizeByMaxMin(newSize, k, minAllocatedResources)
		return jastifiedNewSize, fmt.Sprintf("the current number of replicas is bigger than the preferred max replica number in this cluster (%v), so make %v request (%s) bigger (%v → %v)", s.preferredReplicaNumUpperLimit, k, containerName, resourceRequest.MilliValue(), jastifiedNewSize), nil
	}

	if replicaNum <= s.minimumMinReplicas {
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
		jastified := s.justifyNewSizeByMaxMin(newSize, k, minAllocatedResources)

		return jastified, fmt.Sprintf("the current number of replicas is equal or smaller than the minimum min replica number in this cluster (%v), so make %v request (%v) smaller (%v → %v) based on VPA suggestion", s.minimumMinReplicas, k, containerName, resourceRequest.MilliValue(), jastified), nil
	}

	// The replica number is OK based on minimumMinReplicas and preferredReplicaNumUpperLimit.

	if !isMultipleContainersPod {
		// nothing else to do for a single container Pod.
		return s.justifyNewSizeByMaxMin(resourceRequest.MilliValue(), k, minAllocatedResources), "", nil
	}

	targetUtilizationValue, err := hpaservice.GetHPATargetValue(ctx, hpa, containerName, k)
	if err != nil {
		return 0, "", fmt.Errorf("get the target value from HPA: %w", err)
	}

	upperUtilization := math.Ceil((float64(recommendedResourceRequest.MilliValue()) / float64(resourceRequest.MilliValue())) * 100)
	if targetUtilizationValue > int32(upperUtilization) {
		// upperUtilization is less than targetUtilizationValue, which seems weird in normal cases.
		// In this case, most likely the container size is unbalanced. (= we need multi-container specific optimization)
		// So, for example, when app:istio use the resource in the ratio of 1:5, but the resource request is 1:1,
		// the resource given to istio is always wasted. (since HPA is always kicked by the resource utilization of app)
		//
		// And this case, reducing the resource request of container in this kind of weird situation
		// so that the upper usage will be the target usage.
		newSize := int64(float64(recommendedResourceRequest.MilliValue()) * 100.0 / float64(targetUtilizationValue))
		jastified := s.justifyNewSizeByMaxMin(newSize, k, minAllocatedResources)
		return jastified, fmt.Sprintf("the current resource utilization (%v) is too small and it's due to unbalanced container size, so make %v request (%v) smaller (%v → %v) based on VPA's recommendation and HPA target utilization %v%%", int(upperUtilization), k, containerName, resourceRequest.MilliValue(), jastified, targetUtilizationValue), nil
	}

	// Just keep the current resource request.
	// Only do justification.
	return s.justifyNewSizeByMaxMin(resourceRequest.MilliValue(), k, minAllocatedResources), "", nil
}

func (s *Service) justifyNewSizeByMaxMin(newSizeMilli int64, k corev1.ResourceName, minAllocatedResources corev1.ResourceList) int64 {
	max := s.maxResourceSize[k]
	min := minAllocatedResources[k]

	// Bigger min requirement is used.
	if min.Cmp(s.minResourceSize[k]) < 0 {
		// s.minResourceSize[k] is bigger than minAllocatedResources[k]
		min = s.minResourceSize[k]
	}

	if newSizeMilli > max.MilliValue() {
		return max.MilliValue()
	} else if newSizeMilli < min.MilliValue() {
		return min.MilliValue()
	}

	return newSizeMilli
}

func (s *Service) updateHPARecommendation(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32, now time.Time) (*v1beta3.Tortoise, error) {
	var err error
	tortoise, err = s.updateHPATargetUtilizationRecommendations(ctx, tortoise, hpa, replicaNum)
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
	if tortoise.Status.TortoisePhase == v1beta3.TortoisePhaseEmergency || tortoise.Status.TortoisePhase == v1beta3.TortoisePhaseBackToNormal {
		// If the update mode is emergency or backtonormal, we don't update any recommendation.
		// This is because the replica number goes up during the emergency mode,
		// - the recommendation of min/max replicas would be broken by unusual high number of replicas.
		// - the recommendation of target utilization would be broken by unusual lower resource utilization.
		// - the recommendation of VPA would be broken by unusual lower resource utilization.
		log.FromContext(ctx).Info("The recommendation of minReplica/maxReplica is not updated because of the emergency mode")
		return tortoise, nil
	}

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
	currentReplica := float64(replicaNum)
	min, err := s.updateReplicasRecommendation(int32(math.Ceil(currentReplica*s.minReplicasFactor)), tortoise.Status.Recommendations.Horizontal.MinReplicas, now, s.minimumMinReplicas)
	if err != nil {
		return tortoise, fmt.Errorf("update MinReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MinReplicas = min
	max, err := s.updateReplicasRecommendation(int32(math.Ceil(currentReplica*s.maxReplicasFactor)), tortoise.Status.Recommendations.Horizontal.MaxReplicas, now, int32(float64(s.minimumMinReplicas)*s.maxReplicasFactor/s.minReplicasFactor))
	if err != nil {
		return tortoise, fmt.Errorf("update MaxReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MaxReplicas = max

	return tortoise, nil
}

func findSlotInReplicasRecommendation(recommendations []v1beta3.ReplicasRecommendation, now time.Time) (int, error) {
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
		// shouldn't happen unless someone directly modifies the status.
		return -1, errors.New("no recommendation slot")
	}

	return index, nil
}

// updateMinReplicasRecommendation replaces value if the value is higher than the current value.
func (s *Service) updateReplicasRecommendation(value int32, recommendations []v1beta3.ReplicasRecommendation, now time.Time, min int32) ([]v1beta3.ReplicasRecommendation, error) {
	// find the corresponding recommendations.
	index, err := findSlotInReplicasRecommendation(recommendations, now)
	if err != nil {
		return recommendations, err
	}

	if value < s.minimumMinReplicas {
		value = min
	}

	timeBiasedRecommendation := recommendations[index].Value
	if now.Sub(recommendations[index].UpdatedAt.Time).Hours() >= 23 {
		// only if the recommendation is not updated within 24 hours, we give the time bias
		// so that the past recommendation is decreased a bit and the current recommendation likely replaces it.
		timeBiasedRecommendation = int32(math.Trunc(float64(recommendations[index].Value) * 0.95))
	}

	if value > timeBiasedRecommendation {
		recommendations[index].Value = value
	} else {
		recommendations[index].Value = timeBiasedRecommendation
	}

	recommendations[index].UpdatedAt = metav1.NewTime(now)

	return recommendations, nil
}

func (s *Service) updateHPATargetUtilizationRecommendations(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32) (*v1beta3.Tortoise, error) {
	logger := log.FromContext(ctx)
	if replicaNum == s.maximumMaxReplica {
		// We skip generating HPA recommendations if the current replica number is equal to the maximumMaxReplica
		// because HPA recommendation would be not valid in this case
		// and, either way, editing HPA would not change any situation because the replica number is already at the maximum.
		//
		// This situation should be rare because the replica number shouldn't reach the maximumMaxReplica in normal situation.
		logger.Error(nil, "The recommendation of HPA is not updated because the current replica number is equal to the maximumMaxReplica", "current replica number", replicaNum, "maximumMaxReplica", s.maximumMaxReplica)

		// We still update VPA recommendations because VPA recommendations are not affected by the replica number
		// and hopefully making the container bigger would help the situation.
		return tortoise, nil
	}

	requestMap := map[string]map[corev1.ResourceName]resource.Quantity{}
	for _, r := range tortoise.Status.Conditions.ContainerResourceRequests {
		requestMap[r.ContainerName] = map[corev1.ResourceName]resource.Quantity{}
		for resourcename, value := range r.Resource {
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
			logger.Error(nil, fmt.Sprintf("no resource request on the container %s", r.ContainerName))
			continue
		}
		for k, p := range r.Policy {
			if p != v1beta3.AutoscalingTypeHorizontal {
				// nothing to do.
				continue
			}

			req, ok := reqmap[k]
			if !ok {
				logger.Error(nil, fmt.Sprintf("no %s request on the container %s", k, r.ContainerName))
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
			reason := ""
			if currentTargetValue > int32(upperUsage) {
				// upperUsage is less than targetValue.
				// This case, there're some scenarios:
				// - the container size is unbalanced. (one resource is very bigger than its consumption)
				// - hitting minReplicas.
				//
				// And this case, rather than changing the target value, we'd like to change the container size.
				recommendedTargetUtilization[k] = currentTargetValue // no change (except the current value exceeds maximumTargetResourceUtilization)
				reason = "the current resource utilization is too small and it's due to unbalanced container size or minReplicas, so keep the current target utilization"
			} else {
				newRecom := updateRecommendedContainerBasedMetric(int32(upperUsage), currentTargetValue)
				if newRecom <= 0 || newRecom > 100 {
					logger.Error(nil, "generated recommended HPA target utilization is invalid, fallback to the current target value", "current target utilization", currentTargetValue, "recommended target utilization", newRecom, "upper usage", upperUsage, "container name", r.ContainerName, "resource name", k)
					newRecom = currentTargetValue
					reason = "the generated recommended HPA target utilization is invalid, fallback to the current target value"
				} else {
					reason = "generated recommendation is valid"
				}

				recommendedTargetUtilization[k] = newRecom
			}
			if recommendedTargetUtilization[k] > s.maximumTargetResourceUtilization {
				reason = "the generated recommended HPA target utilization is too high, fallback to the upper target utilization"
				recommendedTargetUtilization[k] = s.maximumTargetResourceUtilization
			}
			if recommendedTargetUtilization[k] < s.minimumTargetResourceUtilization {
				reason = "the generated recommended HPA target utilization is too low, fallback to the lower target utilization"
				recommendedTargetUtilization[k] = s.minimumTargetResourceUtilization
			}

			if currentTargetValue != recommendedTargetUtilization[k] {
				s.eventRecorder.Event(tortoise, corev1.EventTypeNormal, event.RecommendationUpdated, fmt.Sprintf("The recommendation of HPA %v target utilization (%v) in Tortoise status is updated (%v%% → %v%%)", k, r.ContainerName, currentTargetValue, recommendedTargetUtilization[k]))
			} else {
				logger.Info("The recommendation of the container is not updated", "container name", r.ContainerName, "resource name", k, "reason", fmt.Sprintf("HPA target utilization %v%% → %v%%", currentTargetValue, recommendedTargetUtilization[k]))
			}

			logger.Info("HPA target utilization recommendation is generated", "current target utilization", currentTargetValue, "recommended target utilization", recommendedTargetUtilization[k], "upper usage", upperUsage, "container name", r.ContainerName, "resource name", k, "reason", reason)
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
