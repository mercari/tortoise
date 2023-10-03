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

package v1beta1

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var tortoiselog = logf.Log.WithName("tortoise-resource")
var ClientService *service

func (r *Tortoise) SetupWebhookWithManager(mgr ctrl.Manager) error {
	ClientService = newService(mgr.GetClient())
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-autoscaling-mercari-com-v1beta1-tortoise,mutating=true,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1beta1,name=mtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Tortoise{}

const TortoiseDefaultHPANamePrefix = "tortoise-hpa-"

func TortoiseDefaultHPAName(tortoiseName string) string {
	return TortoiseDefaultHPANamePrefix + tortoiseName
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Tortoise) Default() {
	tortoiselog.Info("default", "name", r.Name)

	if r.Spec.UpdateMode == "" {
		r.Spec.UpdateMode = UpdateModeOff
	}
	if r.Spec.DeletionPolicy == "" {
		r.Spec.DeletionPolicy = DeletionPolicyDeleteAll
	}

	if r.Spec.TargetRefs.ScaleTargetRef.Kind == "Deployment" {
		// TODO: do the same validation for other resources.
		d, err := ClientService.GetDeploymentOnTortoise(context.Background(), r)
		if err != nil {
			tortoiselog.Error(err, "failed to get deployment")
			return
		}

		if len(d.Spec.Template.Spec.Containers) != len(r.Spec.ResourcePolicy) {
			for _, c := range d.Spec.Template.Spec.Containers {
				policyExist := false
				for _, p := range r.Spec.ResourcePolicy {
					if c.Name == p.ContainerName {
						policyExist = true
						break
					}
				}
				if !policyExist {
					r.Spec.ResourcePolicy = append(r.Spec.ResourcePolicy, ContainerResourcePolicy{
						ContainerName:     c.Name,
						AutoscalingPolicy: map[v1.ResourceName]AutoscalingType{},
					})
				}
			}
		}
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

//+kubebuilder:webhook:path=/validate-autoscaling-mercari-com-v1beta1-tortoise,mutating=false,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1beta1,name=vtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Tortoise{}

func validateTortoise(t *Tortoise) error {
	fieldPath := field.NewPath("spec")

	if t.Spec.TargetRefs.ScaleTargetRef.Name == "" {
		return fmt.Errorf("%s: shouldn't be empty", fieldPath.Child("targetRefs", "scaleTargetRef", "name"))
	}
	if t.Spec.TargetRefs.ScaleTargetRef.Kind == "" {
		return fmt.Errorf("%s: shouldn't be empty", fieldPath.Child("targetRefs", "scaleTargetRef", "kind"))
	}

	for _, p := range t.Spec.ResourcePolicy {
		for _, ap := range p.AutoscalingPolicy {
			if ap == AutoscalingTypeHorizontal {
				return nil
			}
		}
	}

	if t.Spec.UpdateMode == UpdateModeEmergency &&
		t.Status.TortoisePhase != TortoisePhaseWorking && t.Status.TortoisePhase != TortoisePhaseEmergency && t.Status.TortoisePhase != TortoisePhaseBackToNormal {
		return fmt.Errorf("%s: emergency mode is only available for tortoises with Running phase", fieldPath.Child("updateMode"))
	}

	return fmt.Errorf("%s: at least one policy should be Horizontal", fieldPath.Child("resourcePolicy", "autoscalingPolicy"))
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Tortoise) ValidateCreate() (admission.Warnings, error) {
	ctx := context.Background()
	tortoiselog.Info("validate create", "name", r.Name)
	fieldPath := field.NewPath("spec")
	if r.Spec.TargetRefs.ScaleTargetRef.Kind != "Deployment" {
		return nil, fmt.Errorf("only deployment is supported in %s", fieldPath.Child("targetRefs", "scaleTargetRef", "kind"))
	}
	if err := validateTortoise(r); err != nil {
		return nil, err
	}

	if r.Spec.TargetRefs.ScaleTargetRef.Kind == "Deployment" {
		// TODO: do the same validation for other resources.
		d, err := ClientService.GetDeploymentOnTortoise(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("failed to get the deployment defined in %s: %w", fieldPath.Child("targetRefs", "scaleTargetRef"), err)
		}

		containers := sets.NewString()
		for _, c := range d.Spec.Template.Spec.Containers {
			containers.Insert(c.Name)
		}

		policies := sets.NewString()
		for _, p := range r.Spec.ResourcePolicy {
			policies.Insert(p.ContainerName)
		}

		noPolicyContainers := containers.Difference(policies)
		if noPolicyContainers.Len() != 0 {
			return nil, fmt.Errorf("%s: tortoise should have the policies for all containers defined in the deployment, but, it doesn't have the policy for the container(s) %v", fieldPath.Child("resourcePolicy"), noPolicyContainers)
		}
		uselessPolicies := policies.Difference(containers)
		if uselessPolicies.Len() != 0 {
			return nil, fmt.Errorf("%s: tortoise should not have the policies for the container(s) which isn't defined in the deployment, but, it have the policy for the container(s) %v", fieldPath.Child("resourcePolicy"), uselessPolicies)
		}
	}

	return nil, validateTortoise(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Tortoise) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	tortoiselog.Info("validate update", "name", r.Name)
	if err := validateTortoise(r); err != nil {
		return nil, err
	}

	oldTortoise, ok := old.(*Tortoise)
	if !ok {
		return nil, errors.New("failed to parse old object to Tortoise")
	}

	fieldPath := field.NewPath("spec")
	if r.Spec.TargetRefs.ScaleTargetRef.Name != oldTortoise.Spec.TargetRefs.ScaleTargetRef.Name {
		return nil, fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "scaleTargetRef", "name"))
	}
	if r.Spec.TargetRefs.ScaleTargetRef.Kind != oldTortoise.Spec.TargetRefs.ScaleTargetRef.Kind {
		return nil, fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "scaleTargetRef", "kind"))
	}
	if r.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
		if oldTortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil || *oldTortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != *r.Spec.TargetRefs.HorizontalPodAutoscalerName {
			// removed or updated.
			return nil, fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"))
		}
	} else {
		if oldTortoise.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
			// newly specified.
			return nil, fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"))
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
func (r *Tortoise) ValidateDelete() (admission.Warnings, error) {
	tortoiselog.Info("validate delete", "name", r.Name)
	return nil, nil
}
