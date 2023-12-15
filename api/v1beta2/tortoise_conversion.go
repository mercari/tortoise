/*
MIT License

Copyright (c) 2023 mercari

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/

package v1beta2

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/mercari/tortoise/api/v1beta3"
)

// ConvertTo converts this CronJob to the Hub version (v1beta3).
func (src *Tortoise) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.Tortoise)
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec = v1beta3.TortoiseSpec{
		TargetRefs: v1beta3.TargetRefs{
			HorizontalPodAutoscalerName: src.Spec.TargetRefs.HorizontalPodAutoscalerName,
			ScaleTargetRef: v1beta3.CrossVersionObjectReference{
				APIVersion: src.Spec.TargetRefs.ScaleTargetRef.APIVersion,
				Kind:       src.Spec.TargetRefs.ScaleTargetRef.Kind,
				Name:       src.Spec.TargetRefs.ScaleTargetRef.Name,
			},
		},
		UpdateMode:        v1beta3.UpdateMode(src.Spec.UpdateMode),
		ResourcePolicy:    containerResourcePolicyConversionToV1Beta3(src.Spec.ResourcePolicy),
		AutoscalingPolicy: containerAutoscalingPolicyConversionToV1Beta3(src.Spec.ResourcePolicy),
		DeletionPolicy:    v1beta3.DeletionPolicy(src.Spec.DeletionPolicy),
	}

	dst.Status = v1beta3.TortoiseStatus{
		TortoisePhase: v1beta3.TortoisePhase(src.Status.TortoisePhase),
		Conditions: v1beta3.Conditions{
			ContainerRecommendationFromVPA: containerRecommendationFromVPAConversionToV1Beta3(src.Status.Conditions.ContainerRecommendationFromVPA),
		},
		Targets: v1beta3.TargetsStatus{
			HorizontalPodAutoscaler: src.Status.Targets.HorizontalPodAutoscaler,
			ScaleTargetRef:          v1beta3.CrossVersionObjectReference(src.Spec.TargetRefs.ScaleTargetRef),
			VerticalPodAutoscalers:  verticalPodAutoscalersConversionToV1Beta3(src.Status.Targets.VerticalPodAutoscalers),
		},
		Recommendations: v1beta3.Recommendations{
			Horizontal: v1beta3.HorizontalRecommendations{
				TargetUtilizations: hPATargetUtilizationRecommendationPerContainerConversionToV1Beta3(src.Status.Recommendations.Horizontal.TargetUtilizations),
				MaxReplicas:        replicasRecommendationConversionToV1Beta3(src.Status.Recommendations.Horizontal.MaxReplicas),
				MinReplicas:        replicasRecommendationConversionToV1Beta3(src.Status.Recommendations.Horizontal.MinReplicas),
			},
			Vertical: v1beta3.VerticalRecommendations{
				ContainerResourceRecommendation: containerResourceRecommendationConversionToV1Beta3(src.Status.Recommendations.Vertical.ContainerResourceRecommendation),
			},
		},
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta3) to this version.
func (dst *Tortoise) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.Tortoise)
	if src.Spec.TargetRefs.ScaleTargetRef.Kind != "Deployment" {
		return fmt.Errorf("scaleTargetRef is not Deployment, but %s which isn't supported in v1alpha1", src.Spec.TargetRefs.ScaleTargetRef.Kind)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = TortoiseSpec{
		TargetRefs: TargetRefs{
			HorizontalPodAutoscalerName: src.Spec.TargetRefs.HorizontalPodAutoscalerName,
			ScaleTargetRef: CrossVersionObjectReference{
				APIVersion: src.Spec.TargetRefs.ScaleTargetRef.APIVersion,
				Kind:       src.Spec.TargetRefs.ScaleTargetRef.Kind,
				Name:       src.Spec.TargetRefs.ScaleTargetRef.Name,
			},
		},
		UpdateMode:     UpdateMode(src.Spec.UpdateMode),
		ResourcePolicy: containerResourcePolicyConversionFromV1Beta3(src.Spec.AutoscalingPolicy, src.Spec.ResourcePolicy),
		DeletionPolicy: DeletionPolicy(src.Spec.DeletionPolicy),
	}

	dst.Status = TortoiseStatus{
		TortoisePhase: TortoisePhase(src.Status.TortoisePhase),
		Conditions: Conditions{
			ContainerRecommendationFromVPA: containerRecommendationFromVPAConversionFromV1Beta3(src.Status.Conditions.ContainerRecommendationFromVPA),
		},
		Targets: TargetsStatus{
			HorizontalPodAutoscaler: src.Status.Targets.HorizontalPodAutoscaler,
			VerticalPodAutoscalers:  verticalPodAutoscalersConversionFromV1Beta3(src.Status.Targets.VerticalPodAutoscalers),
			ScaleTargetRef:          CrossVersionObjectReference(src.Spec.TargetRefs.ScaleTargetRef),
		},
		Recommendations: Recommendations{
			Horizontal: HorizontalRecommendations{
				TargetUtilizations: hPATargetUtilizationRecommendationPerContainerConversionFromV1Beta3(src.Status.Recommendations.Horizontal.TargetUtilizations),
				MaxReplicas:        replicasRecommendationConversionFromV1Beta3(src.Status.Recommendations.Horizontal.MaxReplicas),
				MinReplicas:        replicasRecommendationConversionFromV1Beta3(src.Status.Recommendations.Horizontal.MinReplicas),
			},
			Vertical: VerticalRecommendations{
				ContainerResourceRecommendation: containerResourceRecommendationConversionFromV1Beta3(src.Status.Recommendations.Vertical.ContainerResourceRecommendation),
			},
		},
	}
	return nil
}

func verticalPodAutoscalersConversionFromV1Beta3(vpas []v1beta3.TargetStatusVerticalPodAutoscaler) []TargetStatusVerticalPodAutoscaler {
	converted := make([]TargetStatusVerticalPodAutoscaler, 0, len(vpas))
	for _, vpa := range vpas {
		converted = append(converted, TargetStatusVerticalPodAutoscaler{
			Name: vpa.Name,
			Role: VerticalPodAutoscalerRole(vpa.Role),
		})
	}
	return converted
}

func verticalPodAutoscalersConversionToV1Beta3(vpas []TargetStatusVerticalPodAutoscaler) []v1beta3.TargetStatusVerticalPodAutoscaler {
	converted := make([]v1beta3.TargetStatusVerticalPodAutoscaler, 0, len(vpas))
	for _, vpa := range vpas {
		converted = append(converted, v1beta3.TargetStatusVerticalPodAutoscaler{
			Name: vpa.Name,
			Role: v1beta3.VerticalPodAutoscalerRole(vpa.Role),
		})
	}
	return converted
}

func containerResourceRecommendationConversionFromV1Beta3(resources []v1beta3.RecommendedContainerResources) []RecommendedContainerResources {
	converted := make([]RecommendedContainerResources, 0, len(resources))
	for _, resource := range resources {
		converted = append(converted, RecommendedContainerResources{
			ContainerName:       resource.ContainerName,
			RecommendedResource: resource.RecommendedResource,
		})
	}
	return converted
}

func containerResourceRecommendationConversionToV1Beta3(resources []RecommendedContainerResources) []v1beta3.RecommendedContainerResources {
	converted := make([]v1beta3.RecommendedContainerResources, 0, len(resources))
	for _, resource := range resources {
		converted = append(converted, v1beta3.RecommendedContainerResources{
			ContainerName:       resource.ContainerName,
			RecommendedResource: resource.RecommendedResource,
		})
	}
	return converted
}

func replicasRecommendationConversionFromV1Beta3(recommendations []v1beta3.ReplicasRecommendation) []ReplicasRecommendation {
	converted := make([]ReplicasRecommendation, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, ReplicasRecommendation{
			From:      recommendation.From,
			To:        recommendation.To,
			WeekDay:   recommendation.WeekDay,
			TimeZone:  recommendation.TimeZone,
			Value:     recommendation.Value,
			UpdatedAt: recommendation.UpdatedAt,
		})
	}
	return converted
}

func replicasRecommendationConversionToV1Beta3(recommendations []ReplicasRecommendation) []v1beta3.ReplicasRecommendation {
	converted := make([]v1beta3.ReplicasRecommendation, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, v1beta3.ReplicasRecommendation{
			From:      recommendation.From,
			To:        recommendation.To,
			WeekDay:   recommendation.WeekDay,
			TimeZone:  recommendation.TimeZone,
			Value:     recommendation.Value,
			UpdatedAt: recommendation.UpdatedAt,
		})
	}
	return converted
}

func hPATargetUtilizationRecommendationPerContainerConversionFromV1Beta3(recommendations []v1beta3.HPATargetUtilizationRecommendationPerContainer) []HPATargetUtilizationRecommendationPerContainer {
	converted := make([]HPATargetUtilizationRecommendationPerContainer, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     recommendation.ContainerName,
			TargetUtilization: recommendation.TargetUtilization,
		})
	}
	return converted
}

func hPATargetUtilizationRecommendationPerContainerConversionToV1Beta3(recommendations []HPATargetUtilizationRecommendationPerContainer) []v1beta3.HPATargetUtilizationRecommendationPerContainer {
	converted := make([]v1beta3.HPATargetUtilizationRecommendationPerContainer, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, v1beta3.HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     recommendation.ContainerName,
			TargetUtilization: recommendation.TargetUtilization,
		})
	}
	return converted
}

func containerRecommendationFromVPAConversionFromV1Beta3(conditions []v1beta3.ContainerRecommendationFromVPA) []ContainerRecommendationFromVPA {
	converted := make([]ContainerRecommendationFromVPA, 0, len(conditions))
	for _, condition := range conditions {
		converted = append(converted, ContainerRecommendationFromVPA{
			ContainerName:     condition.ContainerName,
			MaxRecommendation: resourceQuantityMapConversionFromV1Beta3(condition.MaxRecommendation),
			Recommendation:    resourceQuantityMapConversionFromV1Beta3(condition.Recommendation),
		})
	}
	return converted
}

func containerRecommendationFromVPAConversionToV1Beta3(conditions []ContainerRecommendationFromVPA) []v1beta3.ContainerRecommendationFromVPA {
	converted := make([]v1beta3.ContainerRecommendationFromVPA, 0, len(conditions))
	for _, condition := range conditions {
		converted = append(converted, v1beta3.ContainerRecommendationFromVPA{
			ContainerName:     condition.ContainerName,
			MaxRecommendation: resourceQuantityMapConversionToV1Beta3(condition.MaxRecommendation),
			Recommendation:    resourceQuantityMapConversionToV1Beta3(condition.Recommendation),
		})
	}
	return converted
}

func resourceQuantityMapConversionFromV1Beta3(resources map[v1.ResourceName]v1beta3.ResourceQuantity) map[v1.ResourceName]ResourceQuantity {
	converted := make(map[v1.ResourceName]ResourceQuantity, len(resources))
	for k, v := range resources {
		converted[k] = ResourceQuantity(v)
	}
	return converted
}

func resourceQuantityMapConversionToV1Beta3(resources map[v1.ResourceName]ResourceQuantity) map[v1.ResourceName]v1beta3.ResourceQuantity {
	converted := make(map[v1.ResourceName]v1beta3.ResourceQuantity, len(resources))
	for k, v := range resources {
		converted[k] = v1beta3.ResourceQuantity(v)
	}
	return converted
}

func containerResourcePolicyConversionFromV1Beta3(autoscalingPolicy []v1beta3.ContainerAutoscalingPolicy, policies []v1beta3.ContainerResourcePolicy) []ContainerResourcePolicy {
	m := make(map[string]map[v1.ResourceName]v1beta3.AutoscalingType, len(autoscalingPolicy))
	for _, policy := range autoscalingPolicy {
		m[policy.ContainerName] = policy.Policy
	}

	converted := make([]ContainerResourcePolicy, 0, len(policies))
	for _, policy := range policies {
		converted = append(converted, ContainerResourcePolicy{
			ContainerName:               policy.ContainerName,
			MinAllocatedResources:       policy.MinAllocatedResources,
			DeplicatedAutoscalingPolicy: autoscalingPolicyConversionFromV1Beta3(m[policy.ContainerName]),
		})
	}
	return converted
}

func containerResourcePolicyConversionToV1Beta3(policies []ContainerResourcePolicy) []v1beta3.ContainerResourcePolicy {
	converted := make([]v1beta3.ContainerResourcePolicy, 0, len(policies))
	for _, policy := range policies {
		converted = append(converted, v1beta3.ContainerResourcePolicy{
			ContainerName:         policy.ContainerName,
			MinAllocatedResources: policy.MinAllocatedResources,
		})
	}
	return converted
}

func containerAutoscalingPolicyConversionToV1Beta3(policies []ContainerResourcePolicy) []v1beta3.ContainerAutoscalingPolicy {
	converted := make([]v1beta3.ContainerAutoscalingPolicy, 0, len(policies))
	for _, policy := range policies {
		converted = append(converted, v1beta3.ContainerAutoscalingPolicy{
			ContainerName: policy.ContainerName,
			Policy:        autoscalingPolicyConversionToV1Beta3(policy.DeplicatedAutoscalingPolicy),
		})
	}
	return converted
}

func autoscalingPolicyConversionFromV1Beta3(policies map[v1.ResourceName]v1beta3.AutoscalingType) map[v1.ResourceName]AutoscalingType {
	converted := make(map[v1.ResourceName]AutoscalingType, len(policies))
	for k, v := range policies {
		converted[k] = AutoscalingType(v)
	}
	return converted
}

func autoscalingPolicyConversionToV1Beta3(policies map[v1.ResourceName]AutoscalingType) map[v1.ResourceName]v1beta3.AutoscalingType {
	converted := make(map[v1.ResourceName]v1beta3.AutoscalingType, len(policies))
	for k, v := range policies {
		converted[k] = v1beta3.AutoscalingType(v)
	}
	return converted
}
