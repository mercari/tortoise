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

package v1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/pod"
	"github.com/mercari/tortoise/pkg/tortoise"
)

// Use FailurePolicy=Ignore deliverately because blocking Pod creation is very critical.
// This mutating webhook is for updating Pod's limit only and even if we cannot update Pod, it's not a big deal.
// The resource request is updated by the VPA mutating webhook anyway.
//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups=core,resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io,admissionReviewVersions=v1
// Memo: ^ I had to change the path from /mutate-core-v1-pod to /mutate--v1-pod because the former was causing an error in the test.
// I guess kubebuilder doesn't handle core type correctly.

func New(
	tortoiseService *tortoise.Service,
	podService *pod.Service,
) *PodWebhook {
	return &PodWebhook{
		tortoiseService: tortoiseService,
		podService:      podService,
	}
}

type PodWebhook struct {
	tortoiseService *tortoise.Service
	podService      *pod.Service
}

var _ admission.CustomDefaulter = &PodWebhook{}

// Default implements admission.CustomDefaulter so a webhook will be registered for the type
func (h *PodWebhook) Default(ctx context.Context, obj runtime.Object) error {
	pod := obj.(*v1.Pod)

	deploymentName, err := h.podService.GetDeploymentForPod(pod)
	if err != nil {
		// Block updating HPA may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get deployment for pod in the Pod mutating webhook", "pod", klog.KObj(pod))
		return nil
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if deploymentName == "" {
		// This Pod isn't managed by any deployment.
		pod.Annotations[annotation.PodMutationAnnotation] = "this pod is not managed by deployment"
		return nil
	}

	tl, err := h.tortoiseService.ListTortoise(ctx, pod.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		// Block updating HPA may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get tortoise for mutating webhook of Pod", "pod", klog.KObj(pod))
		return nil
	}
	if len(tl.Items) == 0 {
		// This namespace don't have any tortoise and thus this HPA isn't managed by tortoise.
		return nil
	}

	var tortoise *v1beta3.Tortoise
	for _, t := range tl.Items {
		if t.Status.Targets.ScaleTargetRef.Name == deploymentName {
			tortoise = t.DeepCopy()
			break
		}
	}
	if tortoise == nil {
		// This Pod isn't managed by any tortoise.
		pod.Annotations[annotation.PodMutationAnnotation] = "this pod is not managed by tortoise"
		return nil
	}

	if tortoise.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// DryRun, don't update Pod
		pod.Annotations[annotation.PodMutationAnnotation] = fmt.Sprintf("this pod is not mutated by tortoise (%s) because the tortoise's update mode is off", tortoise.Name)
		return nil
	}

	if disabled, reason := h.tortoiseService.IsChangeApplicationDisabled(tortoise); disabled {
		// Global disable mode or namespace exclusion is enabled - don't update Pod
		pod.Annotations[annotation.PodMutationAnnotation] = fmt.Sprintf("this pod is not mutated by tortoise (%s) because %s is enabled", tortoise.Name, reason)
		return nil
	}

	h.podService.ModifyPodSpecResource(&pod.Spec, tortoise)
	pod.Annotations[annotation.PodMutationAnnotation] = fmt.Sprintf("this pod is mutated by tortoise (%s)", tortoise.Name)

	return nil
}
