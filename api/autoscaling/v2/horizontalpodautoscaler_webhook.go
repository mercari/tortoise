/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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

	hpa, err = h.hpaService.ChangeHPAFromTortoiseRecommendation(t, hpa, time.Now())
	if err != nil {
		// Block updating HPA may be critical. Just ignore it with error logs.
		klog.ErrorS(err, "failed to get tortoise for mutating webhook of HPA", "hpa", klog.KObj(hpa), "tortoise", tortoiseName)
		return nil
	}

	return nil
}
