package utils

import (
	"github.com/mercari/tortoise/api/v1beta3"
)

type TortoiseBuilder struct {
	tortoise *v1beta3.Tortoise
}

func NewTortoiseBuilder() *TortoiseBuilder {
	return &TortoiseBuilder{
		tortoise: &v1beta3.Tortoise{},
	}
}

func (b *TortoiseBuilder) SetName(name string) *TortoiseBuilder {
	b.tortoise.ObjectMeta.Name = name
	return b
}

func (b *TortoiseBuilder) SetNamespace(namespace string) *TortoiseBuilder {
	b.tortoise.ObjectMeta.Namespace = namespace
	return b
}

func (b *TortoiseBuilder) SetTargetRefs(targetRefs v1beta3.TargetRefs) *TortoiseBuilder {
	b.tortoise.Spec.TargetRefs = targetRefs
	return b
}
func (b *TortoiseBuilder) SetDeletionPolicy(policy v1beta3.DeletionPolicy) *TortoiseBuilder {
	b.tortoise.Spec.DeletionPolicy = policy
	return b
}

func (b *TortoiseBuilder) AddAutoscalingPolicy(p v1beta3.ContainerAutoscalingPolicy) *TortoiseBuilder {
	b.tortoise.Status.AutoscalingPolicy = append(b.tortoise.Status.AutoscalingPolicy, p)
	return b
}

func (b *TortoiseBuilder) SetUpdateMode(updateMode v1beta3.UpdateMode) *TortoiseBuilder {
	b.tortoise.Spec.UpdateMode = updateMode
	return b
}

func (b *TortoiseBuilder) AddResourcePolicy(resourcePolicy v1beta3.ContainerResourcePolicy) *TortoiseBuilder {
	b.tortoise.Spec.ResourcePolicy = append(b.tortoise.Spec.ResourcePolicy, resourcePolicy)
	return b
}

func (b *TortoiseBuilder) SetTortoisePhase(phase v1beta3.TortoisePhase) *TortoiseBuilder {
	b.tortoise.Status.TortoisePhase = phase
	return b
}

func (b *TortoiseBuilder) AddContainerRecommendationFromVPA(recomFromVPA v1beta3.ContainerRecommendationFromVPA) *TortoiseBuilder {
	b.tortoise.Status.Conditions.ContainerRecommendationFromVPA = append(b.tortoise.Status.Conditions.ContainerRecommendationFromVPA, recomFromVPA)
	return b
}

func (b *TortoiseBuilder) AddContainerResourceRequests(actualContainerResource v1beta3.ContainerResourceRequests) *TortoiseBuilder {
	b.tortoise.Status.Conditions.ContainerResourceRequests = append(b.tortoise.Status.Conditions.ContainerResourceRequests, actualContainerResource)
	return b
}

func (b *TortoiseBuilder) AddTortoiseConditions(condition v1beta3.TortoiseCondition) *TortoiseBuilder {
	b.tortoise.Status.Conditions.TortoiseConditions = append(b.tortoise.Status.Conditions.TortoiseConditions, condition)
	return b
}

func (b *TortoiseBuilder) SetRecommendations(recommendations v1beta3.Recommendations) *TortoiseBuilder {
	b.tortoise.Status.Recommendations = recommendations
	return b
}

func (b *TortoiseBuilder) SetTargetsStatus(targetsStatus v1beta3.TargetsStatus) *TortoiseBuilder {
	b.tortoise.Status.Targets = targetsStatus
	return b
}

func (b *TortoiseBuilder) Build() *v1beta3.Tortoise {
	return b.tortoise
}
