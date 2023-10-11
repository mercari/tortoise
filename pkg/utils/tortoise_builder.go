package utils

import (
	"github.com/mercari/tortoise/api/v1beta2"
)

type TortoiseBuilder struct {
	tortoise *v1beta2.Tortoise
}

func NewTortoiseBuilder() *TortoiseBuilder {
	return &TortoiseBuilder{
		tortoise: &v1beta2.Tortoise{},
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

func (b *TortoiseBuilder) SetTargetRefs(targetRefs v1beta2.TargetRefs) *TortoiseBuilder {
	b.tortoise.Spec.TargetRefs = targetRefs
	return b
}
func (b *TortoiseBuilder) SetDeletionPolicy(policy v1beta2.DeletionPolicy) *TortoiseBuilder {
	b.tortoise.Spec.DeletionPolicy = policy
	return b
}

func (b *TortoiseBuilder) SetUpdateMode(updateMode v1beta2.UpdateMode) *TortoiseBuilder {
	b.tortoise.Spec.UpdateMode = updateMode
	return b
}

func (b *TortoiseBuilder) AddResourcePolicy(resourcePolicy v1beta2.ContainerResourcePolicy) *TortoiseBuilder {
	b.tortoise.Spec.ResourcePolicy = append(b.tortoise.Spec.ResourcePolicy, resourcePolicy)
	return b
}

func (b *TortoiseBuilder) SetTortoisePhase(phase v1beta2.TortoisePhase) *TortoiseBuilder {
	b.tortoise.Status.TortoisePhase = phase
	return b
}

func (b *TortoiseBuilder) AddCondition(condition v1beta2.ContainerRecommendationFromVPA) *TortoiseBuilder {
	b.tortoise.Status.Conditions.ContainerRecommendationFromVPA = append(b.tortoise.Status.Conditions.ContainerRecommendationFromVPA, condition)
	return b
}

func (b *TortoiseBuilder) SetRecommendations(recommendations v1beta2.Recommendations) *TortoiseBuilder {
	b.tortoise.Status.Recommendations = recommendations
	return b
}

func (b *TortoiseBuilder) SetTargetsStatus(targetsStatus v1beta2.TargetsStatus) *TortoiseBuilder {
	b.tortoise.Status.Targets = targetsStatus
	return b
}

func (b *TortoiseBuilder) Build() *v1beta2.Tortoise {
	return b.tortoise
}
