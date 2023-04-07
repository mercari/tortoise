/*
MIT License

Copyright (c) 2023 kouzoh

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
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/tortoise"
	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"time"
)

// log is for logging in this package.
var horizontalpodautoscalerlog = logf.Log.WithName("horizontalpodautoscaler-resource")

//+kubebuilder:webhook:path=/mutate-autoscaling-v2-horizontalpodautoscaler,mutating=true,failurePolicy=fail,sideEffects=None,groups=autoscaling,resources=horizontalpodautoscalers,verbs=create;update,versions=v2,name=mhorizontalpodautoscaler.kb.io,admissionReviewVersions=v1

type HPAWebhook struct {
	tortoiseService tortoise.Service
	hpaService      hpa.Client
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
		klog.ErrorS(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa), "tortoise", tortoiseName)
		return nil
	}

	hpa, _, err = h.hpaService.ChangeHPAFromTortoiseRecommendation(t, hpa, time.Now())
	if err != nil {
		// Block updating HPA may be critical. Just ignore it with error logs.
		klog.ErrorS(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa), "tortoise", tortoiseName)
		return nil
	}

	return nil
}
