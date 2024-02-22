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

package v1beta3

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/mercari/tortoise/pkg/annotation"
)

// log is for logging in this package.
var tortoiselog = ctrl.Log.WithName("tortoise-resource")
var ClientService *service

func (r *Tortoise) SetupWebhookWithManager(mgr ctrl.Manager) error {
	ClientService = newService(mgr.GetClient())
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-autoscaling-mercari-com-v1beta3-tortoise,mutating=true,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1beta3,name=mtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Tortoise{}

const TortoiseDefaultHPANamePrefix = "tortoise-hpa-"

func TortoiseDefaultHPAName(tortoiseName string) string {
	return TortoiseDefaultHPANamePrefix + tortoiseName
}

func (r *Tortoise) defaultAutoscalingPolicy() {
	ctx := context.Background()

	if len(r.Spec.AutoscalingPolicy) == 0 {
		return
	}

	// TODO: support other resources.
	if r.Spec.TargetRefs.ScaleTargetRef.Kind == "Deployment" {
		d, err := ClientService.GetDeploymentOnTortoise(ctx, r)
		if err != nil {
			tortoiselog.Error(err, "failed to get deployment")
			return
		}

		containers := d.Spec.Template.Spec.DeepCopy().Containers
		if d.Spec.Template.Annotations != nil {
			if v, ok := d.Spec.Template.Annotations[annotation.IstioSidecarInjectionAnnotation]; ok && v == "true" {
				// If the deployment has the sidecar injection annotation, the Pods will have the sidecar container in addition.
				containers = append(d.Spec.Template.Spec.Containers, v1.Container{
					Name: "istio-proxy",
				})
			}
		}

		if len(containers) != len(r.Spec.AutoscalingPolicy) {
			for _, c := range containers {
				policyExist := false
				for _, p := range r.Spec.AutoscalingPolicy {
					if c.Name == p.ContainerName {
						policyExist = true
						break
					}
				}
				if !policyExist {
					r.Spec.AutoscalingPolicy = append(r.Spec.AutoscalingPolicy, ContainerAutoscalingPolicy{
						ContainerName: c.Name,
						Policy:        map[v1.ResourceName]AutoscalingType{},
					})
				}
			}
		}

		// the default policy is Off
		for i := range r.Spec.AutoscalingPolicy {
			_, ok := r.Spec.AutoscalingPolicy[i].Policy[v1.ResourceCPU]
			if !ok {
				r.Spec.AutoscalingPolicy[i].Policy[v1.ResourceCPU] = AutoscalingTypeOff
			}
			_, ok = r.Spec.AutoscalingPolicy[i].Policy[v1.ResourceMemory]
			if !ok {
				r.Spec.AutoscalingPolicy[i].Policy[v1.ResourceMemory] = AutoscalingTypeOff
			}
		}
	}
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Tortoise) Default() {
	tortoiselog.Info("default", "name", r.Name)

	if r.Spec.UpdateMode == "" {
		r.Spec.UpdateMode = UpdateModeOff
	}
	if r.Spec.DeletionPolicy == "" {
		r.Spec.DeletionPolicy = DeletionPolicyNoDelete
	}

	r.defaultAutoscalingPolicy()
}

//+kubebuilder:webhook:path=/validate-autoscaling-mercari-com-v1beta3-tortoise,mutating=false,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1beta3,name=vtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Tortoise{}

func hasHorizontal(tortoise *Tortoise) bool {
	for _, r := range tortoise.Spec.AutoscalingPolicy {
		for _, p := range r.Policy {
			if p == AutoscalingTypeHorizontal {
				return true
			}
		}
	}
	return false
}

func validateTortoise(t *Tortoise) error {
	fieldPath := field.NewPath("spec")
	if t.Spec.TargetRefs.ScaleTargetRef.Kind == "" {
		return fmt.Errorf("%s: shouldn't be empty", fieldPath.Child("targetRefs", "scaleTargetRef", "kind"))
	}

	if t.Spec.TargetRefs.ScaleTargetRef.Kind != "Deployment" {
		return fmt.Errorf("%s: only Deployment is supported now", fieldPath.Child("targetRefs", "scaleTargetRef", "kind"))
	}

	if t.Spec.TargetRefs.ScaleTargetRef.Name == "" {
		return fmt.Errorf("%s: shouldn't be empty", fieldPath.Child("targetRefs", "scaleTargetRef", "name"))
	}

	if t.Spec.UpdateMode == UpdateModeEmergency &&
		t.Status.TortoisePhase != TortoisePhaseWorking && t.Status.TortoisePhase != TortoisePhaseEmergency && t.Status.TortoisePhase != TortoisePhaseBackToNormal {
		return fmt.Errorf("%s: emergency mode is only available for tortoises with Running phase", fieldPath.Child("updateMode"))
	}

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Tortoise) ValidateCreate() (admission.Warnings, error) {
	ctx := context.Background()
	tortoiselog.Info("validate create", "name", r.Name)
	fieldPath := field.NewPath("spec")
	if r.Spec.TargetRefs.ScaleTargetRef.Kind != "Deployment" {
		return nil, fmt.Errorf("only deployment is supported in %s at the moment", fieldPath.Child("targetRefs", "scaleTargetRef", "kind"))
	}

	if r.Spec.TargetRefs.ScaleTargetRef.Kind == "Deployment" {
		// TODO: do the same validation for other resources.
		d, err := ClientService.GetDeploymentOnTortoise(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("failed to get the deployment defined in %s: %w", fieldPath.Child("targetRefs", "scaleTargetRef"), err)
		}

		containersInDP := sets.New[string]()
		for _, c := range d.Spec.Template.Spec.Containers {
			containersInDP.Insert(c.Name)
		}

		if d.Spec.Template.Annotations != nil {
			if v, ok := d.Spec.Template.Annotations[annotation.IstioSidecarInjectionAnnotation]; ok && v == "true" {
				// If the deployment has the sidecar injection annotation, the Pods will have the sidecar container in addition.
				containersInDP.Insert("istio-proxy")
			}
		}

		containerWithPolicy := sets.New[string]()
		for _, p := range r.Spec.AutoscalingPolicy {
			containerWithPolicy.Insert(p.ContainerName)
		}

		uselessPolicies := containerWithPolicy.Difference(containersInDP)
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
			// newly specified or updated.
			return nil, fmt.Errorf("%s: immutable field get changed", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"))
		}
	}

	if hasHorizontal(oldTortoise) && !hasHorizontal(r) {
		if r.Spec.DeletionPolicy == DeletionPolicyNoDelete {
			// The old one has horizontal, but the new one doesn't have any.
			return nil, fmt.Errorf("%s: no horizontal policy exists. It will cause the deletion of HPA and you need to specify DeleteAll to allow the deletion.", fieldPath.Child("targetRefs", "resourcePolicy", "autoscalingPolicy"))
		}

		if r.Spec.TargetRefs.HorizontalPodAutoscalerName != nil {
			return nil, fmt.Errorf("%s: no horizontal policy exists. It will cause the deletion of HPA and you need to remove horizontalPodAutoscalerName to allow the deletion.", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"))
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
