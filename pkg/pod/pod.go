package pod

import (
	"fmt"
	"math"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerfetcher "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/target/controller_fetcher"
	"k8s.io/utils/ptr"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/features"
	"github.com/mercari/tortoise/pkg/utils"
)

type Service struct {
	// For example, if it's 3 and Pod's resource request is 100m, the limit will be changed to 300m.
	resourceLimitMultiplier       map[string]int64
	minimumCPULimit               resource.Quantity
	controllerFetcher             controllerfetcher.ControllerFetcher
	goMemLimitModificationEnabled bool
}

func New(
	resourceLimitMultiplier map[string]int64,
	minimumCPULimit string,
	cf controllerfetcher.ControllerFetcher,
	featureFlags []features.FeatureFlag,
) (*Service, error) {
	if minimumCPULimit == "" {
		minimumCPULimit = "0"
	}
	minCPULim := resource.MustParse(minimumCPULimit)
	return &Service{
		resourceLimitMultiplier:       resourceLimitMultiplier,
		minimumCPULimit:               minCPULim,
		controllerFetcher:             cf,
		goMemLimitModificationEnabled: features.Contains(featureFlags, features.GoMemLimitModificationEnabled),
	}, nil
}

func (s *Service) ModifyPodTemplateResource(podTemplate *v1.PodTemplateSpec, t *v1beta3.Tortoise, opts ...ModifyPodSpecResourceOption) {
	s.ModifyPodSpecResource(&podTemplate.Spec, t, opts...)

	// Update istio sidecar resource requests based on the tortoise.Status.Conditions.ContainerResourceRequests
	// since ModifyPodSpecResource doesn't update the istio annotations.
	if podTemplate.Annotations == nil {
		return
	}
	if podTemplate.Annotations[annotation.IstioSidecarInjectionAnnotation] != "true" {
		return
	}

	// Update resource requests based on the tortoise.Status.Conditions.ContainerResourceRequests
	for _, k := range []v1.ResourceName{v1.ResourceCPU, v1.ResourceMemory} {
		newReq, ok := utils.GetRequestFromTortoise(t, "istio-proxy", k)
		if !ok {
			continue
		}

		if k == v1.ResourceCPU {
			oldCPUReq, ok := podTemplate.Annotations[annotation.IstioSidecarProxyCPUAnnotation]
			oldCPULim, ok2 := podTemplate.Annotations[annotation.IstioSidecarProxyCPULimitAnnotation]
			if ok && ok2 {
				oldCPUReqQuantity, err := resource.ParseQuantity(oldCPUReq)
				if err != nil {
					continue
				}

				oldCPULimQuantity, err := resource.ParseQuantity(oldCPULim)
				if err != nil {
					continue
				}

				if containsOption(opts, NoScaleDown) && newReq.Cmp(oldCPUReqQuantity) < 0 {
					// If NoScaleDown option is specified, don't scale down the resource request.
					continue
				}

				ratio := float64(newReq.MilliValue()) / float64(oldCPUReqQuantity.MilliValue())
				podTemplate.Annotations[annotation.IstioSidecarProxyCPUAnnotation] = newReq.String()
				podTemplate.Annotations[annotation.IstioSidecarProxyCPULimitAnnotation] = resource.NewMilliQuantity(int64(float64(oldCPULimQuantity.MilliValue())*ratio), oldCPULimQuantity.Format).String()
			}
		}

		if k == v1.ResourceMemory {
			oldMemReq, ok := podTemplate.Annotations[annotation.IstioSidecarProxyMemoryAnnotation]
			oldMemLim, ok2 := podTemplate.Annotations[annotation.IstioSidecarProxyMemoryLimitAnnotation]
			if ok && ok2 {
				oldMemReqQuantity, err := resource.ParseQuantity(oldMemReq)
				if err != nil {
					continue
				}

				oldMemLimQuantity, err := resource.ParseQuantity(oldMemLim)
				if err != nil {
					continue
				}

				if containsOption(opts, NoScaleDown) && newReq.Cmp(oldMemReqQuantity) < 0 {
					// If NoScaleDown option is specified, don't scale down the resource request.
					continue
				}

				ratio := float64(newReq.MilliValue()) / float64(oldMemReqQuantity.MilliValue())
				podTemplate.Annotations[annotation.IstioSidecarProxyMemoryAnnotation] = newReq.String()
				podTemplate.Annotations[annotation.IstioSidecarProxyMemoryLimitAnnotation] = resource.NewMilliQuantity(int64(float64(oldMemLimQuantity.MilliValue())*ratio), oldMemLimQuantity.Format).String()
			}
		}
	}
}

type ModifyPodSpecResourceOption string

var (
	NoScaleDown ModifyPodSpecResourceOption = "NoScaleDown"
)

func (s *Service) ModifyPodSpecResource(podSpec *v1.PodSpec, t *v1beta3.Tortoise, opts ...ModifyPodSpecResourceOption) {
	if t.Spec.UpdateMode == v1beta3.UpdateModeOff ||
		t.Status.TortoisePhase == "" ||
		t.Status.TortoisePhase == v1beta3.TortoisePhaseInitializing ||
		t.Status.TortoisePhase == v1beta3.TortoisePhaseGatheringData {
		return
	}

	oldRequestsMap := map[containerNameAndResource]resource.Quantity{}
	// For example, if the resource request is changed 100m → 200m, 2 will be stored.
	requestChangeRatio := map[containerNameAndResource]float64{}
	newRequestsMap := map[containerNameAndResource]resource.Quantity{}

	// Update resource requests based on the tortoise.Status.Conditions.ContainerResourceRequests
	for i, container := range podSpec.Containers {
		for k, oldReq := range container.Resources.Requests {
			newReq, ok := utils.GetRequestFromTortoise(t, container.Name, k)
			if !ok {
				// Unchange, just store the old value as a new value
				newReq = oldReq
			}
			if containsOption(opts, NoScaleDown) && newReq.Cmp(oldReq) < 0 {
				// If NoScaleDown option is specified, don't scale down the resource request.
				newReq = oldReq
			}
			oldRequestsMap[containerNameAndResource{containerName: container.Name, resourceName: k}] = oldReq
			newRequestsMap[containerNameAndResource{containerName: container.Name, resourceName: k}] = newReq
			podSpec.Containers[i].Resources.Requests[k] = newReq
			requestChangeRatio[containerNameAndResource{containerName: container.Name, resourceName: k}] = float64(newReq.MilliValue()) / float64(oldReq.MilliValue())
		}
	}

	// Update resource limits
	for i, container := range podSpec.Containers {
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
			podSpec.Containers[i].Resources.Limits[k] = *newLim
		}
	}

	// Update GOMEMLIMIT and GOMAXPROCS
	for i, container := range podSpec.Containers {
		for j, env := range container.Env {
			if env.Name == "GOMAXPROCS" {
				// e.g., If CPU is increased twice, GOMAXPROCS should be doubled.
				changeRatio, ok := requestChangeRatio[containerNameAndResource{
					containerName: container.Name,
					resourceName:  v1.ResourceCPU,
				}]
				if !ok {
					continue
				}
				if len(env.Value) == 0 {
					// Probably it's defined through the configmap.
					continue
				}
				oldNum, err := strconv.Atoi(env.Value)
				if err != nil {
					// invalid GOMAXPROCS, skip
					continue
				}
				newUncapedNum := float64(oldNum) * changeRatio
				// GOMAXPROCS should be an integer.
				newNum := int(math.Ceil(newUncapedNum))
				podSpec.Containers[i].Env[j].Value = strconv.Itoa(newNum)

			}

			if !s.goMemLimitModificationEnabled {
				// don't modify GOMEMLIMIT
				continue
			}

			if env.Name == "GOMEMLIMIT" {
				changeRatio, ok := requestChangeRatio[containerNameAndResource{
					containerName: container.Name,
					resourceName:  v1.ResourceMemory,
				}]
				if !ok {
					continue
				}
				val := env.Value
				if len(val) == 0 {
					// Probably it's defined through the configmap.
					continue
				}
				last := val[len(val)-1]
				if last >= '0' && last <= '9' {
					// OK
				} else if last == 'B' {
					// It should end with B.
					val = val[:len(val)-1]
				} else {
					// invalid GOMEMLIMIT, skip
					continue
				}

				oldNum, err := resource.ParseQuantity(val)
				if err != nil {
					// invalid GOMEMLIMIT, skip
					continue
				}
				// See GOMEMLIMIT's format: https://pkg.go.dev/runtime#hdr-Environment_Variables
				newNum := int(float64(oldNum.Value()) * changeRatio)
				podSpec.Containers[i].Env[j].Value = strconv.Itoa(newNum)
			}
		}
	}
}

func (s *Service) GetDeploymentForPod(pod *v1.Pod) (string, error) {
	var ownerRefrence *metav1.OwnerReference
	for i := range pod.OwnerReferences {
		r := pod.OwnerReferences[i]
		if r.Controller != nil && *r.Controller {
			ownerRefrence = &r
		}
	}
	if ownerRefrence == nil {
		// If the pod has no ownerReference, it cannot be under Tortoise.
		return "", nil
	}

	if ownerRefrence.Kind != "ReplicaSet" {
		// Tortoise only supports Deployment for now, and ReplicaSet is the only controller that can own a pod in this case.
		return "", nil
	}

	k := &controllerfetcher.ControllerKeyWithAPIVersion{
		ControllerKey: controllerfetcher.ControllerKey{
			Namespace: pod.Namespace,
			Kind:      ownerRefrence.Kind,
			Name:      ownerRefrence.Name,
		},
		ApiVersion: ownerRefrence.APIVersion,
	}

	topController, err := s.controllerFetcher.FindTopMostWellKnownOrScalable(k)
	if err != nil {
		return "", fmt.Errorf("failed to find top most well known or scalable controller: %v", err)
	}

	if topController.Kind != "Deployment" {
		// Tortoise only supports Deployment for now.
		return "", nil
	}

	return topController.Name, nil
}

type containerNameAndResource struct {
	containerName string
	resourceName  v1.ResourceName
}

func containsOption(opts []ModifyPodSpecResourceOption, opt ModifyPodSpecResourceOption) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}
