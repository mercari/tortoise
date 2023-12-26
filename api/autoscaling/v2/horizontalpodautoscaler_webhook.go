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

package autoscalingv2

import (
	"context"
	"fmt"
	"time"

	v2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/tortoise"
)

//+kubebuilder:webhook:path=/mutate-autoscaling-v2-horizontalpodautoscaler,mutating=true,failurePolicy=fail,sideEffects=None,groups=autoscaling,resources=horizontalpodautoscalers,verbs=create;update,versions=v2,name=mhorizontalpodautoscaler.kb.io,admissionReviewVersions=v1

func New(tortoiseService *tortoise.Service, hpaService *hpa.Service) *HPAWebhook {
	return &HPAWebhook{
		tortoiseService: tortoiseService,
		hpaService:      hpaService,
	}
}

type HPAWebhook struct {
	tortoiseService *tortoise.Service
	hpaService      *hpa.Service
}

var _ admission.CustomDefaulter = &HPAWebhook{}

// Default implements admission.CustomDefaulter so a webhook will be registered for the type
func (h *HPAWebhook) Default(ctx context.Context, obj runtime.Object) error {
	hpa := obj.(*v2.HorizontalPodAutoscaler)
	tortoiseName, ok := hpa.GetAnnotations()[annotation.TortoiseNameAnnotation]
	if !ok {
		// not managed by tortoise
		return nil
	}

	t, err := h.tortoiseService.GetTortoise(ctx, types.NamespacedName{
		Namespace: hpa.Namespace,
		Name:      tortoiseName,
	})
	if err != nil {
		// Block updating HPA may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa), "tortoise", tortoiseName)
		return nil
	}

	if t.Status.Targets.HorizontalPodAutoscaler != hpa.Name {
		// should not reach here
		return nil
	}

	if t.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// DryRun, don't update HPA
		return nil
	}

	hpa, _, err = h.hpaService.ChangeHPAFromTortoiseRecommendation(t, hpa, time.Now(), false) // we don't need to record metrics.
	if err != nil {
		// Block updating HPA may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa), "tortoise", tortoiseName)
		return nil
	}

	return nil
}

//+kubebuilder:webhook:path=/validate-autoscaling-v2-horizontalpodautoscaler,mutating=false,failurePolicy=fail,sideEffects=None,groups=autoscaling,resources=horizontalpodautoscalers,verbs=delete,versions=v2,name=mhorizontalpodautoscaler.kb.io,admissionReviewVersions=v1

var _ admission.CustomValidator = &HPAWebhook{}

// ValidateCreate validates the object on creation.
// The optional warnings will be added to the response as warning messages.
// Return an error if the object is invalid.
func (*HPAWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

// ValidateUpdate validates the object on update.
// The optional warnings will be added to the response as warning messages.
// Return an error if the object is invalid.
func (*HPAWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

// ValidateDelete validates the object on deletion.
// The optional warnings will be added to the response as warning messages.
// Return an error if the object is invalid.
func (h *HPAWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	hpa := obj.(*v2.HorizontalPodAutoscaler)
	tortoiseName, ok := hpa.GetAnnotations()[annotation.TortoiseNameAnnotation]
	if !ok {
		// not managed by tortoise
		return nil, nil
	}

	t, err := h.tortoiseService.GetTortoise(ctx, types.NamespacedName{
		Namespace: hpa.Namespace,
		Name:      tortoiseName,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// expected scenario - tortoise is deleted before HPA is deleted.
			return nil, nil
		}
		// unknown error
		return nil, fmt.Errorf("failed to get tortoise(%s) for mutating webhook of HPA(%s/%s): %w", tortoiseName, hpa.Namespace, hpa.Name, err)
	}
	if !t.ObjectMeta.DeletionTimestamp.IsZero() {
		// expected scenario - tortoise is being deleted before HPA is deleted.
		return nil, nil
	}

	if t.Status.Targets.HorizontalPodAutoscaler != hpa.Name {
		// should not reach here
		return nil, nil
	}

	message := fmt.Sprintf("HPA(%s/%s) is being deleted while Tortoise(%s) is running. Please delete Tortoise first", hpa.Namespace, hpa.Name, tortoiseName)

	if t.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// DryRun - don't block the deletion of HPA, but emit warning.
		return []string{message}, nil
	}

	return nil, fmt.Errorf(message)
}
