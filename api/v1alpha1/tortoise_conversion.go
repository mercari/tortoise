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

package v1alpha1

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/mercari/tortoise/api/v1beta2"
)

// ConvertTo converts this CronJob to the Hub version (v1beta2).
func (src *Tortoise) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.Tortoise)
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec = v1beta2.TortoiseSpec{
		TargetRefs: v1beta2.TargetRefs{
			HorizontalPodAutoscalerName: src.Spec.TargetRefs.HorizontalPodAutoscalerName,
			ScaleTargetRef: v1beta2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       src.Spec.TargetRefs.DeploymentName,
			},
		},
		UpdateMode:     v1beta2.UpdateMode(src.Spec.UpdateMode),
		ResourcePolicy: containerResourcePolicyConversionToV1Beta1(src.Spec.ResourcePolicy),
		DeletionPolicy: v1beta2.DeletionPolicy(src.Spec.DeletionPolicy),
	}

	dst.Status = v1beta2.TortoiseStatus{
		TortoisePhase: v1beta2.TortoisePhase(src.Status.TortoisePhase),
		Conditions: v1beta2.Conditions{
			ContainerRecommendationFromVPA: containerRecommendationFromVPAConversionToV1Beta1(src.Status.Conditions.ContainerRecommendationFromVPA),
		},
		Targets: v1beta2.TargetsStatus{
			HorizontalPodAutoscaler: src.Status.Targets.HorizontalPodAutoscaler,
			ScaleTargetRef: v1beta2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       src.Status.Targets.Deployment,
			},
			VerticalPodAutoscalers: verticalPodAutoscalersConversionToV1Beta1(src.Status.Targets.VerticalPodAutoscalers),
		},
	}
	if src.Status.Recommendations.Horizontal != nil {
		dst.Status = v1beta2.TortoiseStatus{
			Recommendations: v1beta2.Recommendations{
				Horizontal: v1beta2.HorizontalRecommendations{
					TargetUtilizations: hPATargetUtilizationRecommendationPerContainerConversionToV1Beta1(src.Status.Recommendations.Horizontal.TargetUtilizations),
					MaxReplicas:        replicasRecommendationConversionToV1Beta1(src.Status.Recommendations.Horizontal.MaxReplicas),
					MinReplicas:        replicasRecommendationConversionToV1Beta1(src.Status.Recommendations.Horizontal.MinReplicas),
				},
			},
		}
	}
	if src.Status.Recommendations.Vertical != nil {
		dst.Status = v1beta2.TortoiseStatus{
			Recommendations: v1beta2.Recommendations{
				Vertical: v1beta2.VerticalRecommendations{
					ContainerResourceRecommendation: containerResourceRecommendationConversionToV1Beta1(src.Status.Recommendations.Vertical.ContainerResourceRecommendation),
				},
			},
		}
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version.
func (dst *Tortoise) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.Tortoise)
	if src.Spec.TargetRefs.ScaleTargetRef.Kind != "Deployment" {
		return fmt.Errorf("scaleTargetRef is not Deployment, but %s which isn't supported in v1alpha1", src.Spec.TargetRefs.ScaleTargetRef.Kind)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = TortoiseSpec{
		TargetRefs: TargetRefs{
			HorizontalPodAutoscalerName: src.Spec.TargetRefs.HorizontalPodAutoscalerName,
			DeploymentName:              src.Spec.TargetRefs.ScaleTargetRef.Name,
		},
		UpdateMode:     UpdateMode(src.Spec.UpdateMode),
		ResourcePolicy: containerResourcePolicyConversionFromV1Beta1(src.Spec.ResourcePolicy),
		DeletionPolicy: DeletionPolicy(src.Spec.DeletionPolicy),
	}

	dst.Status = TortoiseStatus{
		TortoisePhase: TortoisePhase(src.Status.TortoisePhase),
		Conditions: Conditions{
			ContainerRecommendationFromVPA: containerRecommendationFromVPAConversionFromV1Beta1(src.Status.Conditions.ContainerRecommendationFromVPA),
		},
		Targets: TargetsStatus{
			HorizontalPodAutoscaler: src.Status.Targets.HorizontalPodAutoscaler,
			Deployment:              src.Status.Targets.ScaleTargetRef.Name,
			VerticalPodAutoscalers:  verticalPodAutoscalersConversionFromV1Beta1(src.Status.Targets.VerticalPodAutoscalers),
		},
		Recommendations: Recommendations{
			Horizontal: &HorizontalRecommendations{
				TargetUtilizations: hPATargetUtilizationRecommendationPerContainerConversionFromV1Beta1(src.Status.Recommendations.Horizontal.TargetUtilizations),
				MaxReplicas:        replicasRecommendationConversionFromV1Beta1(src.Status.Recommendations.Horizontal.MaxReplicas),
				MinReplicas:        replicasRecommendationConversionFromV1Beta1(src.Status.Recommendations.Horizontal.MinReplicas),
			},
			Vertical: &VerticalRecommendations{
				ContainerResourceRecommendation: containerResourceRecommendationConversionFromV1Beta1(src.Status.Recommendations.Vertical.ContainerResourceRecommendation),
			},
		},
	}
	return nil
}

func verticalPodAutoscalersConversionFromV1Beta1(vpas []v1beta2.TargetStatusVerticalPodAutoscaler) []TargetStatusVerticalPodAutoscaler {
	converted := make([]TargetStatusVerticalPodAutoscaler, 0, len(vpas))
	for _, vpa := range vpas {
		converted = append(converted, TargetStatusVerticalPodAutoscaler{
			Name: vpa.Name,
			Role: VerticalPodAutoscalerRole(vpa.Role),
		})
	}
	return converted
}

func verticalPodAutoscalersConversionToV1Beta1(vpas []TargetStatusVerticalPodAutoscaler) []v1beta2.TargetStatusVerticalPodAutoscaler {
	converted := make([]v1beta2.TargetStatusVerticalPodAutoscaler, 0, len(vpas))
	for _, vpa := range vpas {
		converted = append(converted, v1beta2.TargetStatusVerticalPodAutoscaler{
			Name: vpa.Name,
			Role: v1beta2.VerticalPodAutoscalerRole(vpa.Role),
		})
	}
	return converted
}

func containerResourceRecommendationConversionFromV1Beta1(resources []v1beta2.RecommendedContainerResources) []RecommendedContainerResources {
	converted := make([]RecommendedContainerResources, 0, len(resources))
	for _, resource := range resources {
		converted = append(converted, RecommendedContainerResources{
			ContainerName:       resource.ContainerName,
			RecommendedResource: resource.RecommendedResource,
		})
	}
	return converted
}

func containerResourceRecommendationConversionToV1Beta1(resources []RecommendedContainerResources) []v1beta2.RecommendedContainerResources {
	converted := make([]v1beta2.RecommendedContainerResources, 0, len(resources))
	for _, resource := range resources {
		converted = append(converted, v1beta2.RecommendedContainerResources{
			ContainerName:       resource.ContainerName,
			RecommendedResource: resource.RecommendedResource,
		})
	}
	return converted
}

func replicasRecommendationConversionFromV1Beta1(recommendations []v1beta2.ReplicasRecommendation) []ReplicasRecommendation {
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

func replicasRecommendationConversionToV1Beta1(recommendations []ReplicasRecommendation) []v1beta2.ReplicasRecommendation {
	converted := make([]v1beta2.ReplicasRecommendation, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, v1beta2.ReplicasRecommendation{
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

func hPATargetUtilizationRecommendationPerContainerConversionFromV1Beta1(recommendations []v1beta2.HPATargetUtilizationRecommendationPerContainer) []HPATargetUtilizationRecommendationPerContainer {
	converted := make([]HPATargetUtilizationRecommendationPerContainer, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     recommendation.ContainerName,
			TargetUtilization: recommendation.TargetUtilization,
		})
	}
	return converted
}

func hPATargetUtilizationRecommendationPerContainerConversionToV1Beta1(recommendations []HPATargetUtilizationRecommendationPerContainer) []v1beta2.HPATargetUtilizationRecommendationPerContainer {
	converted := make([]v1beta2.HPATargetUtilizationRecommendationPerContainer, 0, len(recommendations))
	for _, recommendation := range recommendations {
		converted = append(converted, v1beta2.HPATargetUtilizationRecommendationPerContainer{
			ContainerName:     recommendation.ContainerName,
			TargetUtilization: recommendation.TargetUtilization,
		})
	}
	return converted
}

func containerRecommendationFromVPAConversionFromV1Beta1(conditions []v1beta2.ContainerRecommendationFromVPA) []ContainerRecommendationFromVPA {
	converted := make([]ContainerRecommendationFromVPA, 0, len(conditions))
	for _, condition := range conditions {
		converted = append(converted, ContainerRecommendationFromVPA{
			ContainerName:     condition.ContainerName,
			MaxRecommendation: resourceQuantityMapConversionFromV1Beta1(condition.MaxRecommendation),
			Recommendation:    resourceQuantityMapConversionFromV1Beta1(condition.Recommendation),
		})
	}
	return converted
}

func containerRecommendationFromVPAConversionToV1Beta1(conditions []ContainerRecommendationFromVPA) []v1beta2.ContainerRecommendationFromVPA {
	converted := make([]v1beta2.ContainerRecommendationFromVPA, 0, len(conditions))
	for _, condition := range conditions {
		converted = append(converted, v1beta2.ContainerRecommendationFromVPA{
			ContainerName:     condition.ContainerName,
			MaxRecommendation: resourceQuantityMapConversionToV1Beta1(condition.MaxRecommendation),
			Recommendation:    resourceQuantityMapConversionToV1Beta1(condition.Recommendation),
		})
	}
	return converted
}

func resourceQuantityMapConversionFromV1Beta1(resources map[v1.ResourceName]v1beta2.ResourceQuantity) map[v1.ResourceName]ResourceQuantity {
	converted := make(map[v1.ResourceName]ResourceQuantity, len(resources))
	for k, v := range resources {
		converted[k] = ResourceQuantity(v)
	}
	return converted
}

func resourceQuantityMapConversionToV1Beta1(resources map[v1.ResourceName]ResourceQuantity) map[v1.ResourceName]v1beta2.ResourceQuantity {
	converted := make(map[v1.ResourceName]v1beta2.ResourceQuantity, len(resources))
	for k, v := range resources {
		converted[k] = v1beta2.ResourceQuantity(v)
	}
	return converted
}

func containerResourcePolicyConversionFromV1Beta1(policies []v1beta2.ContainerResourcePolicy) []ContainerResourcePolicy {
	converted := make([]ContainerResourcePolicy, 0, len(policies))
	for _, policy := range policies {
		converted = append(converted, ContainerResourcePolicy{
			ContainerName:         policy.ContainerName,
			MinAllocatedResources: policy.MinAllocatedResources,
			AutoscalingPolicy:     autoscalingPolicyConversionFromV1Beta1(policy.AutoscalingPolicy),
		})
	}
	return converted
}

func containerResourcePolicyConversionToV1Beta1(policies []ContainerResourcePolicy) []v1beta2.ContainerResourcePolicy {
	converted := make([]v1beta2.ContainerResourcePolicy, 0, len(policies))
	for _, policy := range policies {
		converted = append(converted, v1beta2.ContainerResourcePolicy{
			ContainerName:         policy.ContainerName,
			MinAllocatedResources: policy.MinAllocatedResources,
			AutoscalingPolicy:     autoscalingPolicyConversionToV1Beta1(policy.AutoscalingPolicy),
		})
	}
	return converted
}

func autoscalingPolicyConversionFromV1Beta1(policies map[v1.ResourceName]v1beta2.AutoscalingType) map[v1.ResourceName]AutoscalingType {
	converted := make(map[v1.ResourceName]AutoscalingType, len(policies))
	for k, v := range policies {
		converted[k] = AutoscalingType(v)
	}
	return converted
}

func autoscalingPolicyConversionToV1Beta1(policies map[v1.ResourceName]AutoscalingType) map[v1.ResourceName]v1beta2.AutoscalingType {
	converted := make(map[v1.ResourceName]v1beta2.AutoscalingType, len(policies))
	for k, v := range policies {
		converted[k] = v1beta2.AutoscalingType(v)
	}
	return converted
}
