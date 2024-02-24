package pod

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/mercari/tortoise/api/v1beta3"
)

type Service struct {
	// For example, if it's 3 and Pod's resource request is 100m, the limit will be changed to 300m.
	resourceLimitMultiplier map[string]int64
	minimumCPULimit         resource.Quantity
}

func New(
	resourceLimitMultiplier map[string]int64,
	MinimumCPULimit string,
) (*Service, error) {
	minCPULim := resource.MustParse(MinimumCPULimit)
	return &Service{
		resourceLimitMultiplier: resourceLimitMultiplier,
		minimumCPULimit:         minCPULim,
	}, nil
}

func (s *Service) ModifyPodResource(pod *v1.Pod, t *v1beta3.Tortoise) {
	if t.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// DryRun, don't update Pod
		return
	}

	oldRequestsMap := map[containerNameAndResource]resource.Quantity{}
	newRequestsMap := map[containerNameAndResource]resource.Quantity{}

	// Update resource requests based on the tortoise.Status.Conditions.ContainerResourceRequests
	for i, container := range pod.Spec.Containers {
		for k, oldReq := range container.Resources.Requests {
			oldRequestsMap[containerNameAndResource{containerName: container.Name, resourceName: k}] = oldReq

			newReq, ok := getRequestFromTortoise(t, container.Name, k)
			if !ok {
				// Unchange, just store the old value as a new value
				newRequestsMap[containerNameAndResource{containerName: container.Name, resourceName: k}] = oldReq
				continue
			}
			pod.Spec.Containers[i].Resources.Requests[k] = newReq
			newRequestsMap[containerNameAndResource{containerName: container.Name, resourceName: k}] = newReq
		}
	}

	// Update resource limits
	for i, container := range pod.Spec.Containers {
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(v1.ResourceList)
		}

		for k, oldLimit := range container.Resources.Limits {
			// Keeping limit proportional to request.

			key := containerNameAndResource{containerName: container.Name, resourceName: k}
			oldReq, ok := oldRequestsMap[key]
			if !ok {
				// There's no request for this limit, so we cannot calculate the new limit.
				continue
			}
			oldRatio := float64(oldLimit.MilliValue()) / float64(oldReq.MilliValue())
			if multiplier, ok := s.resourceLimitMultiplier[string(k)]; ok {
				if oldRatio < float64(multiplier) {
					// Previous limit is lower than expected.
					oldRatio = float64(multiplier)
				}
			}

			newReq := newRequestsMap[key]
			newLim := resource.NewMilliQuantity(int64(float64(newReq.MilliValue())*oldRatio), oldLimit.Format)
			if k == v1.ResourceCPU && newLim.Cmp(s.minimumCPULimit) < 0 {
				newLim = ptr.To(s.minimumCPULimit.DeepCopy())
			}
			pod.Spec.Containers[i].Resources.Limits[k] = *newLim
		}
	}
}

type containerNameAndResource struct {
	containerName string
	resourceName  v1.ResourceName
}

// getRequestFromTortoise returns the resource request from the tortoise.Status.Conditions.ContainerResourceRequests.
func getRequestFromTortoise(t *v1beta3.Tortoise, containerName string, resourceName v1.ResourceName) (resource.Quantity, bool) {
	for _, req := range t.Status.Conditions.ContainerResourceRequests {
		if req.ContainerName == containerName {
			rec, ok := req.Resource[resourceName]
			return rec, ok
		}
	}

	return resource.Quantity{}, false
}
