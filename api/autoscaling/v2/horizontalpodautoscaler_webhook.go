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
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/mercari/tortoise/api/v1beta3"
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
	tl, err := h.tortoiseService.ListTortoise(ctx, hpa.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		// Block updating HPA may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa))
		return nil
	}
	if len(tl.Items) == 0 {
		// This namespace don't have any tortoise and thus this HPA isn't managed by tortoise.
		return nil
	}

	var tortoise *v1beta3.Tortoise
	for _, t := range tl.Items {
		if t.Status.Targets.HorizontalPodAutoscaler == hpa.Name {
			tortoise = t.DeepCopy()
			break
		}
	}
	if tortoise == nil {
		// This HPA isn't managed by any tortoise.
		return nil
	}

	if tortoise.Spec.UpdateMode == v1beta3.UpdateModeOff {
		// DryRun, don't update HPA
		return nil
	}

	if disabled, _ := h.tortoiseService.IsChangeApplicationDisabled(tortoise); disabled {
		// Global disable mode or namespace exclusion is enabled - don't update HPA
		return nil
	}

	// tortoisePhase may be changed in ChangeHPAFromTortoiseRecommendation, so we need to get it before calling it.
	tortoisePhase := tortoise.Status.TortoisePhase

	modifiedhpa, _, err := h.hpaService.ChangeHPAFromTortoiseRecommendation(tortoise, hpa.DeepCopy(), time.Now(), false) // we don't need to record metrics.
	if err != nil {
		// Block updating HPA may be critical. Just ignore it with error logs.
		log.FromContext(ctx).Error(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa), "tortoise", tortoise.Name)
		return nil
	}

	if tortoisePhase == v1beta3.TortoisePhaseBackToNormal {
		// If we want to overwrite minReplicas and maxReplicas, it'd be complicated.
		hpa.Spec.Metrics = modifiedhpa.Spec.Metrics
	} else {
		hpa.Spec.Metrics = modifiedhpa.Spec.Metrics
		hpa.Spec.MinReplicas = modifiedhpa.Spec.MinReplicas
		hpa.Spec.MaxReplicas = modifiedhpa.Spec.MaxReplicas
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
	tl, err := h.tortoiseService.ListTortoise(ctx, hpa.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// expected scenario - tortoise is deleted before HPA is deleted.
			return nil, nil
		}
		// unknown error
		return nil, fmt.Errorf("failed to list tortoise in the same namespace for mutating webhook of HPA(%s/%s): %w", hpa.Namespace, hpa.Name, err)
	}
	if len(tl.Items) == 0 {
		// expected scenario - tortoise is deleted before HPA is deleted OR this HPA is not managed by tortoise.
		return nil, nil
	}

	var tortoise *v1beta3.Tortoise
	for _, t := range tl.Items {
		if t.Status.Targets.HorizontalPodAutoscaler == hpa.Name {
			tortoise = t.DeepCopy()
			break
		}
	}
	if tortoise == nil {
		// expected scenario - tortoise is deleted before HPA is deleted OR this HPA is not managed by tortoise.
		return nil, nil
	}

	if !tortoise.ObjectMeta.DeletionTimestamp.IsZero() {
		// expected scenario - tortoise is being deleted before HPA is deleted.
		return nil, nil
	}

	return nil, fmt.Errorf("HPA(%s/%s) is being deleted while Tortoise(%s) is running. Please delete Tortoise first", hpa.Namespace, hpa.Name, tortoise.Name)
}
