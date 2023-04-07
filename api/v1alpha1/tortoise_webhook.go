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

package v1alpha1

import (
	"errors"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var tortoiselog = logf.Log.WithName("tortoise-resource")

func (r *Tortoise) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-autoscaling-mercari-com-v1alpha1-tortoise,mutating=true,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1alpha1,name=mtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Tortoise{}

const TortoiseDefaultHPANamePrefix = "tortoise-hpa-"

func TortoiseDefaultHPAName(tortoiseName string) string {
	return TortoiseDefaultHPANamePrefix + tortoiseName
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Tortoise) Default() {
	tortoiselog.Info("default", "name", r.Name)

	if r.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		r.Spec.TargetRefs.HorizontalPodAutoscalerName = pointer.String(TortoiseDefaultHPAName(r.Name))
	}
	if r.Spec.UpdateMode == "" {
		r.Spec.UpdateMode = UpdateModeOff
	}

	for i := range r.Spec.ResourcePolicy {
		_, ok := r.Spec.ResourcePolicy[i].AutoscalingPolicy[v1.ResourceCPU]
		if !ok {
			r.Spec.ResourcePolicy[i].AutoscalingPolicy[v1.ResourceCPU] = AutoscalingTypeHorizontal
		}
		_, ok = r.Spec.ResourcePolicy[i].AutoscalingPolicy[v1.ResourceMemory]
		if !ok {
			r.Spec.ResourcePolicy[i].AutoscalingPolicy[v1.ResourceMemory] = AutoscalingTypeVertical
		}
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-autoscaling-mercari-com-v1alpha1-tortoise,mutating=false,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1alpha1,name=vtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Tortoise{}

func validateTortoise(t *Tortoise) error {
	fieldPath := field.NewPath("spec")

	if t.Spec.TargetRefs.DeploymentName == "" {
		return fmt.Errorf("%s: shouldn't be empty", fieldPath.Child("targetRefs", "deploymentName"))
	}

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Tortoise) ValidateCreate() error {
	tortoiselog.Info("validate create", "name", r.Name)
	return validateTortoise(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Tortoise) ValidateUpdate(old runtime.Object) error {
	tortoiselog.Info("validate update", "name", r.Name)
	if err := validateTortoise(r); err != nil {
		return err
	}

	oldTortoise, ok := old.(*Tortoise)
	if !ok {
		return errors.New("failed to parse old object to Tortoise")
	}

	fieldPath := field.NewPath("spec")
	if r.Spec.TargetRefs.DeploymentName != oldTortoise.Spec.TargetRefs.DeploymentName {
		return fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "deploymentNames"))
	}
	if r.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		if *oldTortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != TortoiseDefaultHPAName(r.Name) {
			return fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"))
		}
	} else {
		if *oldTortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != *r.Spec.TargetRefs.HorizontalPodAutoscalerName {
			return fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"))
		}
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Tortoise) ValidateDelete() error {
	tortoiselog.Info("validate delete", "name", r.Name)
	return nil
}
