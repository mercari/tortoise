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

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
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
	resourceLimitMultiplier map[string]int64,
	minimumCPULimitCores string,
) (*PodWebhook, error) {
	minCPULim := resource.MustParse(minimumCPULimitCores)
	return &PodWebhook{
		tortoiseService:         tortoiseService,
		resourceLimitMultiplier: resourceLimitMultiplier,
		minimumCPULimit:         minCPULim,
	}, nil
}

type PodWebhook struct {
	tortoiseService *tortoise.Service
	// For example, if it's 3 and Pod's resource request is 100m, the limit will be changed to 300m.
	resourceLimitMultiplier map[string]int64
	minimumCPULimit         resource.Quantity
}

var _ admission.CustomDefaulter = &PodWebhook{}

// Default implements admission.CustomDefaulter so a webhook will be registered for the type
func (h *PodWebhook) Default(ctx context.Context, obj runtime.Object) error {
	if len(h.resourceLimitMultiplier) == 0 {
		return nil
	}
	pod := obj.(*v1.Pod)
	tortoiseName, ok := pod.GetAnnotations()[annotation.TortoiseNameAnnotation]
	if !ok {
		// not managed by tortoise
		return nil
	}

	t, err := h.tortoiseService.GetTortoise(ctx, types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      tortoiseName,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.FromContext(ctx).Info("tortoise not found for mutating webhook of Pod", "pod", klog.KObj(pod), "tortoise", tortoiseName)
			return nil
		}
		// Unknown error, but blocking updating Pod may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get tortoise for mutating webhook of Pod", "pod", klog.KObj(pod), "tortoise", tortoiseName)
		return nil
	}

	if t.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// DryRun, don't update Pod
		return nil
	}

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(v1.ResourceList)
		}
		for k := range container.Resources.Requests {
			if _, ok := h.resourceLimitMultiplier[string(k)]; !ok {
				continue
			}

			req := container.Resources.Requests[k].DeepCopy()
			newLimit := resource.NewMilliQuantity(int64(req.MilliValue())*h.resourceLimitMultiplier[string(k)], req.Format)
			if k == v1.ResourceCPU && newLimit.Cmp(h.minimumCPULimit) < 0 {
				newLimit = ptr.To(h.minimumCPULimit.DeepCopy())
			}
			container.Resources.Limits[k] = ptr.Deref(newLimit, container.Resources.Limits[k])
		}
	}

	return nil
}
