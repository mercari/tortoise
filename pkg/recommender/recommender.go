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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/event"
	"github.com/mercari/tortoise/pkg/features"
	hpaservice "github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/utils"
)

type Service struct {
	// configurations
	MaxReplicasRecommendationMultiplier float64
	MinReplicasRecommendationMultiplier float64

	eventRecorder                    record.EventRecorder
	minimumMinReplicas               int32
	maximumTargetResourceUtilization int32
	minimumTargetResourceUtilization int32
	preferredMaxReplicas             int32
	maxResourceSize                  corev1.ResourceList
	// the key is the container name, and "*" is the value for all containers.
	minResourceSizePerContainer map[string]corev1.ResourceList
	maximumMaxReplica           int32
	featureFlags                []features.FeatureFlag
	// maxAllowedScalingDownRatio is the max allowed scaling down ratio.
	// For example, if the current resource request is 100m, the max allowed scaling down ratio is 0.8,
	// the minimum resource request that Tortoise can apply is 80m.
	maxAllowedScalingDownRatio float64

	bufferRatioOnVerticalResource float64
}

func New(
	maxReplicasRecommendationMultiplier float64,
	minReplicasRecommendationMultiplier float64,
	maximumTargetResourceUtilization int,
	minimumTargetResourceUtilization int,
	minimumMinReplicas int,
	preferredMaxReplicas int,
	minCPU string,
	minMemory string,
	minimumCPUPerContainer map[string]string,
	minimumMemoryPerContainer map[string]string,
	maxCPU string,
	maxMemory string,
	maximumMaxReplica int32,
	maxAllowedScalingDownRatio float64,
	bufferRatioOnVerticalResourceRecommendation float64,
	featureFlags []features.FeatureFlag,
	eventRecorder record.EventRecorder,
) *Service {
	minimumCPUPerContainer["*"] = minCPU
	minimumMemoryPerContainer["*"] = minMemory

	minResourceSizePerContainer := map[string]corev1.ResourceList{}
	for containerName, v := range minimumCPUPerContainer {
		minResourceSizePerContainer[containerName] = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(v),
		}
	}
	for containerName, v := range minimumMemoryPerContainer {
		if _, ok := minResourceSizePerContainer[containerName]; !ok {
			minResourceSizePerContainer[containerName] = corev1.ResourceList{}
		}
		minResourceSizePerContainer[containerName][corev1.ResourceMemory] = resource.MustParse(v)
	}

	return &Service{
		eventRecorder:                       eventRecorder,
		MaxReplicasRecommendationMultiplier: maxReplicasRecommendationMultiplier,
		MinReplicasRecommendationMultiplier: minReplicasRecommendationMultiplier,
		maximumTargetResourceUtilization:    int32(maximumTargetResourceUtilization),
		minimumTargetResourceUtilization:    int32(minimumTargetResourceUtilization),
		minimumMinReplicas:                  int32(minimumMinReplicas),
		preferredMaxReplicas:                int32(preferredMaxReplicas),
		minResourceSizePerContainer:         minResourceSizePerContainer,
		maxResourceSize: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(maxCPU),
			corev1.ResourceMemory: resource.MustParse(maxMemory),
		},
		maximumMaxReplica:             maximumMaxReplica,
		featureFlags:                  featureFlags,
		maxAllowedScalingDownRatio:    maxAllowedScalingDownRatio,
		bufferRatioOnVerticalResource: bufferRatioOnVerticalResourceRecommendation,
	}
}

func (s *Service) updateVPARecommendation(ctx context.Context, tortoise *v1beta3.Tortoise, hpa *v2.HorizontalPodAutoscaler, replicaNum int32, now time.Time) (*v1beta3.Tortoise, error) {
	scaledUpBasedOnPreferredMaxReplicas := false
	closeToPreferredMaxReplicas := false
	if hasHorizontal(tortoise) {
		// Handle TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas condition first.
		if replicaNum >= s.preferredMaxReplicas &&
			// If the current replica number is equal to the maximumMaxReplica,
			// increasing the resource request would not change the situation that the replica number is higher than preferredMaxReplicas.
			*hpa.Spec.MinReplicas < replicaNum &&
			features.Contains(s.featureFlags, features.VerticalScalingBasedOnPreferredMaxReplicas) &&
			allowVerticalScalingBasedOnPreferredMaxReplicas(tortoise, now) {

			c := utils.GetTortoiseCondition(tortoise, v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas)
			if c == nil || // no condition yet
				c.Status == v1.ConditionFalse {
				// It's the first time to notice that the current replica number is bigger than the preferred max replica number.
				// First 30min, we don't use VerticalScalingBasedOnPreferredMaxReplicas because this replica increase might be very temporal.
				// So, here we just change the condition to True, but doesn't trigger scaledUpBasedOnPreferredMaxReplicas.
				tortoise = utils.ChangeTortoiseCondition(tortoise, v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas, v1.ConditionTrue, "ScaledUpBasedOnPreferredMaxReplicas", "the current number of replicas is bigger than the preferred max replica number", now)
			} else {
				// We keep increasing the size until we hit the maxResourceSize.
				tortoise = utils.ChangeTortoiseCondition(tortoise, v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas, v1.ConditionTrue, "ScaledUpBasedOnPreferredMaxReplicas", "the current number of replicas is bigger than the preferred max replica number", now)
				scaledUpBasedOnPreferredMaxReplicas = true
			}
		}
		if replicaNum < s.preferredMaxReplicas {
			// Change TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas to False.
			tortoise = utils.ChangeTortoiseCondition(tortoise, v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas, v1.ConditionFalse, "ScaledUpBasedOnPreferredMaxReplicas", "the current number of replicas is not bigger than the preferred max replica number", now)
		}
		if int32(float64(s.preferredMaxReplicas)*0.8) < replicaNum {
			closeToPreferredMaxReplicas = true
		}
	}

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
		for k, perResource := range perContainer.MaxRecommendation {
			recommendationMap[perContainer.ContainerName][k] = perResource.Quantity
		}
	}

	// containerName → MinAllocatedResources
	minAllocatedResourcesMap := map[string]v1.ResourceList{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		minAllocatedResourcesMap[r.ContainerName] = r.MinAllocatedResources
	}

	// containerName → MaxAllocatedResources
	maxAllocatedResourcesMap := map[string]v1.ResourceList{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		maxAllocatedResourcesMap[r.ContainerName] = r.MaxAllocatedResources
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
			var newSize int64
			var reason string
			var err error
			newSize, reason, err = s.calculateBestNewSize(ctx, tortoise, p, r.ContainerName, recom, k, hpa, replicaNum, req, minAllocatedResourcesMap[r.ContainerName], maxAllocatedResourcesMap[r.ContainerName], scaledUpBasedOnPreferredMaxReplicas, closeToPreferredMaxReplicas)
			if err != nil {
				return tortoise, err
			}

			if newSize != req.MilliValue() {
				logger.Info("The recommendation of resource request in Tortoise is updated", "container name", r.ContainerName, "resource name", k, "reason", reason)
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

func allowVerticalScalingBasedOnPreferredMaxReplicas(tortoise *v1beta3.Tortoise, now time.Time) bool {
	for _, c := range tortoise.Status.Conditions.TortoiseConditions {
		if c.Type == v1beta3.TortoiseConditionTypeScaledUpBasedOnPreferredMaxReplicas && c.Status == v1.ConditionTrue {
			if c.LastTransitionTime.Add(30 * time.Minute).After(now) {
				// If the last transition time is within 30 minutes,
				// we don't allow the vertical scaling based on the preferred max replicas.
				return false
			}
		}
	}

	return true
}

// calculateBestNewSize calculates the best new resource request based on the current replica number and the recommended resource request.
// Even if the autoscaling policy is Horizontal, this function may suggest the vertical scaling, see comments in the function.
func (s *Service) calculateBestNewSize(
	ctx context.Context,
	tortoise *v1beta3.Tortoise,
	p v1beta3.AutoscalingType,
	containerName string,
	recommendedResourceRequest resource.Quantity,
	k corev1.ResourceName,
	hpa *v2.HorizontalPodAutoscaler,
	replicaNum int32,
	resourceRequest resource.Quantity,
	minAllocatedResources, maxAllocatedResources corev1.ResourceList,
	scaledUpBasedOnPreferredMaxReplicas, closeToPreferredMaxReplicas bool,
) (int64, string, error) {
	if p == v1beta3.AutoscalingTypeOff {
		// Just keep the current resource request.
		return resourceRequest.MilliValue(), "The autoscaling policy for this resource is Off", nil
	}

	if p == v1beta3.AutoscalingTypeVertical {
		// The user configures Vertical on this container's resource. This is just vertical scaling.
		// Basically we want to reduce the frequency of scaling up/down because vertical scaling has to restart deployment.

		// The ideal size is {VPA recommendation} * (1+buffer).
		idealSize := float64(recommendedResourceRequest.MilliValue()) * (1 + s.bufferRatioOnVerticalResource)
		if idealSize > float64(resourceRequest.MilliValue()) {
			// Scale up always happens when idealSize goes higher than the current resource request.
			// In this case, we don't just apply idealSize, but apply idealSize * (1+buffer)
			// so that we increase the resource request more than actually needed,
			// which reduces the need of scaling up in the future.
			idealSize = idealSize * (1 + s.bufferRatioOnVerticalResource)
			jastified := s.justifyNewSize(resourceRequest.MilliValue(), int64(idealSize), k, minAllocatedResources, maxAllocatedResources, containerName)
			return jastified, fmt.Sprintf("change %v request (%v) (%v → %v) based on VPA suggestion", k, containerName, resourceRequest.MilliValue(), jastified), nil
		}

		// Scale down - we ignore too small scale down to reduce the frequency of restarts.

		// previousIdealSize was the ideal size which was calculated when this resource request was applied.
		previousIdealSize := float64(resourceRequest.MilliValue()) / (1 + s.bufferRatioOnVerticalResource)
		if previousIdealSize*(1-s.bufferRatioOnVerticalResource) > idealSize {
			// The current ideal size is too small campared to the previous ideal size.
			jastified := s.justifyNewSize(resourceRequest.MilliValue(), int64(idealSize), k, minAllocatedResources, maxAllocatedResources, containerName)
			return jastified, fmt.Sprintf("change %v request (%v) (%v → %v) based on VPA suggestion", k, containerName, resourceRequest.MilliValue(), jastified), nil
		}

		return resourceRequest.MilliValue(),
			fmt.Sprintf("Tortoise recommends %v as a new %v request (%v), but it's very small scale down change, so tortoise just ignores it", idealSize, k, containerName),
			nil
	}

	// p == v1beta3.AutoscalingTypeHorizontal

	// When the current replica num is more than or equal to the preferredMaxReplicas,
	// make the container size bigger (just multiple by 1.3) so that the replica number will be descreased.
	//
	// Here also covers the scenario where the current replica num hits MaximumMaxReplicas.
	if scaledUpBasedOnPreferredMaxReplicas {
		// We keep increasing the size until we hit the maxResourceSize.
		newSize := int64(float64(resourceRequest.MilliValue()) * 1.3)
		jastifiedNewSize := s.justifyNewSize(resourceRequest.MilliValue(), newSize, k, minAllocatedResources, maxAllocatedResources, containerName)
		msg := fmt.Sprintf("the current number of replicas (%v) is bigger than the preferred max replica number in this cluster (%v), so make %v request (%s) bigger (%v → %v)", replicaNum, s.preferredMaxReplicas, k, containerName, resourceRequest.MilliValue(), jastifiedNewSize)
		return jastifiedNewSize, msg, nil
	}

	if closeToPreferredMaxReplicas {
		// The current replica number is close or more than preferredMaxReplicas.
		// So, we just keep the current resource request
		// until the replica number goes lower
		// because scaling down the resource request might increase the replica number further more.
		return resourceRequest.MilliValue(), fmt.Sprintf("the current number of replicas is close to the preferred max replica number in this cluster, so keep the current resource request in %s in %s", k, containerName), nil
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
		jastified := s.justifyNewSize(resourceRequest.MilliValue(), newSize, k, minAllocatedResources, maxAllocatedResources, containerName)

		return jastified, fmt.Sprintf("the current number of replicas is equal or smaller than the minimum min replica number in this cluster (%v), so make %v request (%v) smaller (%v → %v) based on VPA suggestion", s.minimumMinReplicas, k, containerName, resourceRequest.MilliValue(), jastified), nil
	}

	// The replica number is OK based on minimumMinReplicas and preferredMaxReplicas.

	if !hasMultipleHorizontal(tortoise) || replicaNum == *hpa.Spec.MinReplicas {
		// Nothing else to do for a single-horizontal Tortoise.
		// Also, if the current replica number is equal to the minReplicas,
		// we don't change the resource request based on the current resource utilization
		// because even if the resource utilization is low, it's due to the minReplicas.
		return s.justifyNewSize(resourceRequest.MilliValue(), resourceRequest.MilliValue(), k, minAllocatedResources, maxAllocatedResources, containerName), "nothing to do", nil
	}

	targetUtilizationValue, err := hpaservice.GetHPATargetValue(ctx, hpa, containerName, k)
	if err != nil {
		return 0, "", fmt.Errorf("get the target value from HPA: %w", err)
	}

	upperUtilization := (float64(recommendedResourceRequest.MilliValue()) / float64(resourceRequest.MilliValue())) * 100
	// If upperUtilization is very close to targetUtilizationValue, we don't have to change the resource request.
	if float64(targetUtilizationValue)*0.9 > upperUtilization {
		// upperUtilization is much less than targetUtilizationValue, which seems weird in normal cases.
		// In this case, most likely the container size is unbalanced. (= we need multi-container specific optimization)
		// So, for example, when app:istio use the resource in the ratio of 1:5, but the resource request is 1:1,
		// the resource given to istio is always wasted. (since HPA is always kicked by the resource utilization of app)
		//
		// And this case, reducing the resource request of container in this kind of weird situation
		// so that the upper usage will be the target usage.
		newSize := int64(float64(recommendedResourceRequest.MilliValue()) * 100.0 / float64(targetUtilizationValue))
		jastified := s.justifyNewSize(resourceRequest.MilliValue(), newSize, k, minAllocatedResources, maxAllocatedResources, containerName)
		return jastified, fmt.Sprintf("the current resource usage (%v, %v%%) is too small and it's due to unbalanced container size, so make %v request (%v) smaller (%v → %v) based on VPA's recommendation and HPA target utilization %v%%", recommendedResourceRequest.MilliValue(), int(upperUtilization), k, containerName, resourceRequest.MilliValue(), jastified, targetUtilizationValue), nil
	}

	// Just keep the current resource request.
	// Only do justification.
	return s.justifyNewSize(resourceRequest.MilliValue(), resourceRequest.MilliValue(), k, minAllocatedResources, maxAllocatedResources, containerName), "nothing to do", nil
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

func hasMultipleHorizontal(t *v1beta3.Tortoise) bool {
	count := 0
	for _, r := range t.Status.AutoscalingPolicy {
		for _, p := range r.Policy {
			if p == v1beta3.AutoscalingTypeHorizontal {
				count++
			}
			if count > 1 {
				return true
			}
		}
	}
	return false
}

func (s *Service) getGlobalMinResourceSize(k corev1.ResourceName, containerName string) resource.Quantity {
	if v, ok := s.minResourceSizePerContainer[containerName]; ok {
		return v[k]
	}

	return s.minResourceSizePerContainer["*"][k]
}

func (s *Service) justifyNewSize(oldSizeMilli, newSizeMilli int64, k corev1.ResourceName, minAllocatedResources, maxAllocatedResources corev1.ResourceList, containerName string) int64 {
	max := maxAllocatedResources[k]
	min := minAllocatedResources[k]

	// Bigger min requirement is used.
	if min.Cmp(s.getGlobalMinResourceSize(k, containerName)) < 0 {
		// s.minResourceSize[k] is bigger than minAllocatedResources[k]
		min = s.getGlobalMinResourceSize(k, containerName)
	}

	// Smaller max requirement is used.
	if max.Cmp(s.maxResourceSize[k]) > 0 || max.IsZero() {
		// s.maxResourceSize[k] is smaller than maxAllocatedResources[k]
		// OR maxAllocatedResources[k] is unset.
		max = s.maxResourceSize[k]
	}

	// If the new size is too small, which isn't acceptable based on the maxAllowedScalingDownRatio.
	// We use oldSizeMilli * s.maxAllowedScalingDownRatio as the new size.
	//
	// So, here if min is smaller than oldSizeMilli * s.maxAllowedScalingDownRatio,
	// we use oldSizeMilli * s.maxAllowedScalingDownRatio as min.
	if min.MilliValue() < int64(float64(oldSizeMilli)*s.maxAllowedScalingDownRatio) {
		min = ptr.Deref(resource.NewMilliQuantity(int64(float64(oldSizeMilli)*s.maxAllowedScalingDownRatio), min.Format), min)
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

	tortoise, err = s.updateVPARecommendation(ctx, tortoise, hpa, replicaNum, now)
	if err != nil {
		return tortoise, fmt.Errorf("update VPA recommendations: %w", err)
	}

	return tortoise, nil
}

func (s *Service) updateHPAMinMaxReplicasRecommendations(tortoise *v1beta3.Tortoise, replicaNum int32, now time.Time) (*v1beta3.Tortoise, error) {
	currentReplica := float64(replicaNum)
	min, err := s.updateReplicasRecommendation(int32(math.Ceil(currentReplica*s.MinReplicasRecommendationMultiplier)), tortoise.Status.Recommendations.Horizontal.MinReplicas, now, s.minimumMinReplicas)
	if err != nil {
		return tortoise, fmt.Errorf("update MinReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MinReplicas = min
	max, err := s.updateReplicasRecommendation(int32(math.Ceil(currentReplica*s.MaxReplicasRecommendationMultiplier)), tortoise.Status.Recommendations.Horizontal.MaxReplicas, now, int32(float64(s.minimumMinReplicas)*s.MaxReplicasRecommendationMultiplier/s.MinReplicasRecommendationMultiplier))
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
				// The metric might be missing because it's not yet synced to the HPA status.
				// We don't want to error out the whole reconciliation loop in this case.
				// Just skip this metric for now.
				logger.V(4).Info("try to find the metric for the container which is configured to be scale by Horizontal, but it's not found", "error", err)
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
