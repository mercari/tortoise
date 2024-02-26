package utils

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mercari/tortoise/api/v1beta3"
)

func ChangeTortoiseCondition(t *v1beta3.Tortoise, conditionType v1beta3.TortoiseConditionType, status corev1.ConditionStatus, reason, message string, now time.Time) *v1beta3.Tortoise {
	for i := range t.Status.Conditions.TortoiseConditions {
		if t.Status.Conditions.TortoiseConditions[i].Type == conditionType {
			t.Status.Conditions.TortoiseConditions[i].Reason = reason
			t.Status.Conditions.TortoiseConditions[i].Message = message
			t.Status.Conditions.TortoiseConditions[i].Status = status
			t.Status.Conditions.TortoiseConditions[i].LastTransitionTime = metav1.NewTime(now)
			t.Status.Conditions.TortoiseConditions[i].LastUpdateTime = metav1.NewTime(now)
			return t
		}
	}
	// add a new condition if not found.
	t.Status.Conditions.TortoiseConditions = append(t.Status.Conditions.TortoiseConditions, v1beta3.TortoiseCondition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(now),
		LastUpdateTime:     metav1.NewTime(now),
	})

	return t
}

func ChangeTortoiseResourcePhase(tortoise *v1beta3.Tortoise, containerName string, rn corev1.ResourceName, now time.Time, phase v1beta3.ContainerResourcePhase) *v1beta3.Tortoise {
	found := false
	for i, p := range tortoise.Status.ContainerResourcePhases {
		if p.ContainerName == containerName {
			tortoise.Status.ContainerResourcePhases[i].ResourcePhases[rn] = v1beta3.ResourcePhase{
				Phase:              phase,
				LastTransitionTime: metav1.NewTime(now),
			}

			found = true
			break
		}
	}
	if !found {
		tortoise.Status.ContainerResourcePhases = append(tortoise.Status.ContainerResourcePhases, v1beta3.ContainerResourcePhases{
			ContainerName: containerName,
			ResourcePhases: map[corev1.ResourceName]v1beta3.ResourcePhase{
				rn: {
					Phase:              phase,
					LastTransitionTime: metav1.NewTime(now),
				},
			},
		})
	}

	return tortoise
}

func RemoveTortoiseResourcePhase(tortoise *v1beta3.Tortoise, containerName string) *v1beta3.Tortoise {
	for i, p := range tortoise.Status.ContainerResourcePhases {
		if p.ContainerName == containerName {
			tortoise.Status.ContainerResourcePhases = append(tortoise.Status.ContainerResourcePhases[:i], tortoise.Status.ContainerResourcePhases[i+1:]...)
			break
		}
	}

	return tortoise
}

// getRequestFromTortoise returns the resource request from the tortoise.Status.Conditions.ContainerResourceRequests.
func GetRequestFromTortoise(t *v1beta3.Tortoise, containerName string, resourceName v1.ResourceName) (resource.Quantity, bool) {
	for _, req := range t.Status.Conditions.ContainerResourceRequests {
		if req.ContainerName == containerName {
			rec, ok := req.Resource[resourceName]
			return rec, ok
		}
	}

	return resource.Quantity{}, false
}
