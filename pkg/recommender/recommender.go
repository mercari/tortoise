package recommender

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mercari/tortoise/pkg/annotation"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/klog/v2"

	corev1 "k8s.io/api/core/v1"

	v1 "k8s.io/api/apps/v1"

	"github.com/mercari/tortoise/api/v1alpha1"
	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Service struct {
	// configurations
	rangeOfMinMaxReplicasRecommendation   time.Duration
	TTLHourOfMinMaxReplicasRecommendation float64
	maxReplicasFactor                     float64
	minReplicasFactor                     float64

	minimumMinReplicas             int32
	upperTargetResourceUtilization int32
	preferredReplicaNumAtPeak      int32
	suggestedResourceSizeAtMax     corev1.ResourceList
}

func New() *Service {
	return &Service{
		// TODO: make them configurable via flag
		rangeOfMinMaxReplicasRecommendation:   1 * time.Hour,
		TTLHourOfMinMaxReplicasRecommendation: 24 * 7 * 4, // 1 month
		maxReplicasFactor:                     2,
		minReplicasFactor:                     0.5,
		upperTargetResourceUtilization:        90,
		minimumMinReplicas:                    3,
		preferredReplicaNumAtPeak:             30,
	}
}

func (s *Service) updateVPARecommendation(tortoise *v1alpha1.Tortoise, deployment *v1.Deployment) (*v1alpha1.Tortoise, error) {
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

	newRecommendations := []v1alpha1.RecommendedContainerResources{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		recommendation := v1alpha1.RecommendedContainerResources{
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

			newSize := req.MilliValue()
			if deployment.Status.Replicas <= s.minimumMinReplicas || p == v1alpha1.AutoscalingTypeVertical {
				recomMap, ok := recommendationMap[r.ContainerName]
				if !ok {
					return nil, fmt.Errorf("no resource recommendation from VPA for the container %s", r.ContainerName)
				}
				recom, ok := recomMap[k]
				if !ok {
					return nil, fmt.Errorf("no %s recommendation from VPA for the container %s", k, r.ContainerName)
				}

				newSize = recom.MilliValue()
			}
			if deployment.Status.Replicas >= s.preferredReplicaNumAtPeak {
				newSize = int64(float64(req.MilliValue()) * 1.1)
			}

			newSize = s.justifyNewSizeByMaxMin(newSize, k, req, r.MinAllocatedResources)
			q := resource.NewMilliQuantity(newSize, req.Format)
			recommendation.RecommendedResource[k] = *q
		}
		newRecommendations = append(newRecommendations, recommendation)
	}

	tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation = newRecommendations

	return tortoise, nil
}

func (s *Service) justifyNewSizeByMaxMin(newSize int64, k corev1.ResourceName, req resource.Quantity, MinAllocatedResources corev1.ResourceList) int64 {
	max := s.suggestedResourceSizeAtMax[k]
	min := MinAllocatedResources[k]

	if req.MilliValue() > max.MilliValue() {
		return req.MilliValue()
	} else if newSize > max.MilliValue() {
		return max.MilliValue()
	} else if newSize < min.MilliValue() {
		return min.MilliValue()
	}

	return newSize
}

func (s *Service) updateHPARecommendation(tortoise *v1alpha1.Tortoise, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment, now time.Time) (*v1alpha1.Tortoise, error) {
	if tortoise.Spec.UpdateMode == v1alpha1.UpdateModeOff {
		// dry-run
		klog.Info("tortoise is dry-run mode", "tortoise", klog.KObj(tortoise))
		return tortoise, nil
	}

	if tortoise.Status.TortoisePhase == v1alpha1.TortoisePhaseGatheringData {
		klog.Info("tortoise is gathering data and unable to recommend", "tortoise", klog.KObj(tortoise))
		return tortoise, nil
	}

	var err error
	tortoise, err = s.updateHPATargetUtilizationRecommendations(tortoise, hpa, deployment)
	if err != nil {
		return nil, fmt.Errorf("update HPA target utilization recommendations: %w", err)
	}
	tortoise, err = s.updateHPAMinMaxReplicasRecommendations(tortoise, deployment, now)
	if err != nil {
		return nil, err
	}

	return tortoise, nil
}

func (s *Service) UpdateRecommendations(tortoise *v1alpha1.Tortoise, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment, now time.Time) (*v1alpha1.Tortoise, error) {
	var err error
	tortoise, err = s.updateHPARecommendation(tortoise, hpa, deployment, now)
	if err != nil {
		return nil, fmt.Errorf("update HPA recommendations: %w", err)
	}
	tortoise, err = s.updateVPARecommendation(tortoise, deployment)
	if err != nil {
		return nil, fmt.Errorf("update VPA recommendations: %w", err)
	}

	return tortoise, nil
}

func (s *Service) updateHPAMinMaxReplicasRecommendations(tortoise *v1alpha1.Tortoise, deployment *v1.Deployment, now time.Time) (*v1alpha1.Tortoise, error) {
	currentReplicaNum := float64(deployment.Status.Replicas)
	min, err := s.updateMaxMinReplicasRecommendation(int32(math.Ceil(currentReplicaNum*s.minReplicasFactor)), tortoise.Status.Recommendations.Horizontal.MinReplicas, now, s.minimumMinReplicas)
	if err != nil {
		return tortoise, fmt.Errorf("update MinReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MinReplicas = min
	max, err := s.updateMaxMinReplicasRecommendation(int32(math.Ceil(currentReplicaNum*s.maxReplicasFactor)), tortoise.Status.Recommendations.Horizontal.MaxReplicas, now, 0)
	if err != nil {
		return tortoise, fmt.Errorf("update MaxReplicas recommendation: %w", err)
	}
	tortoise.Status.Recommendations.Horizontal.MaxReplicas = max

	return tortoise, nil
}

// updateMaxMinReplicasRecommendation replaces value if the value is higher than the current value.
func (s *Service) updateMaxMinReplicasRecommendation(value int32, recommendations []v1alpha1.ReplicasRecommendation, now time.Time, minimum int32) ([]v1alpha1.ReplicasRecommendation, error) {
	// find the corresponding recommendations.
	index := -1
	for i, r := range recommendations {
		if now.Hour() < r.To && now.Hour() >= r.From && now.Weekday() == r.WeekDay {
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

func (s *Service) updateHPATargetUtilizationRecommendations(tortoise *v1alpha1.Tortoise, hpa *v2.HorizontalPodAutoscaler, deployment *v1.Deployment) (*v1alpha1.Tortoise, error) {
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

	newHPATargetUtilizationRecommendationPerContainer := []v1alpha1.HPATargetUtilizationRecommendationPerContainer{}
	for _, r := range tortoise.Spec.ResourcePolicy {
		targetMap := map[corev1.ResourceName]int32{}
		for k, p := range r.AutoscalingPolicy {
			if p == v1alpha1.AutoscalingTypeVertical {
				targetMap[k] = s.upperTargetResourceUtilization
				continue
			}

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

			targetValue, err := getHPATargetValue(hpa, r.ContainerName, k)
			if err != nil {
				return nil, fmt.Errorf("get the target value from HPA: %w", err)
			}

			recomMap, ok := recommendationMap[r.ContainerName]
			if !ok {
				return nil, fmt.Errorf("no resource recommendation from VPA for the container %s", r.ContainerName)
			}
			recom, ok := recomMap[k]
			if !ok {
				return nil, fmt.Errorf("no %s recommendation from VPA for the container %s", k, r.ContainerName)
			}

			targetMap[k] = updateRecommendedContainerBasedMetric(req, targetValue, recom)
		}
		newHPATargetUtilizationRecommendationPerContainer = append(newHPATargetUtilizationRecommendationPerContainer, v1alpha1.HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     r.ContainerName,
			TargetUtilization: targetMap,
		})
	}

	tortoise.Status.Recommendations.Horizontal.TargetUtilizations = newHPATargetUtilizationRecommendationPerContainer

	return tortoise, nil
}

// Currently, only supports:
// - The container resource metric with AverageUtilization.
// - The external metric with AverageUtilization.
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

	var prefix string
	switch k {
	case corev1.ResourceCPU:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation]
	case corev1.ResourceMemory:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation]
	default:
		return 0, fmt.Errorf("non supported resource type: %s", k)
	}
	externalMetricName := prefix + containerName

	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ExternalMetricSourceType {
			continue
		}

		if m.External == nil {
			// shouldn't reach here
			klog.ErrorS(nil, "invalid external metric", klog.KObj(hpa))
			continue
		}

		if m.External.Metric.Name != externalMetricName {
			continue
		}

		return int32(m.External.Target.Value.Value()), nil
	}

	return 0, fmt.Errorf("unsupported hpa")
}

func updateRecommendedContainerBasedMetric(currentResourceReq resource.Quantity, currentTarget int32, recommendationFromVPA resource.Quantity) int32 {
	// TODO: what happens if the resource request get changed?
	// Should we change the phase to GatheringData
	upperUsage := math.Ceil((float64(recommendationFromVPA.MilliValue()) / float64(currentResourceReq.MilliValue())) * 100)
	additionalResource := int32(upperUsage) - currentTarget
	return 100 - additionalResource
}
