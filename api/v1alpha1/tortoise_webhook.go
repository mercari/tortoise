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
	"context"
	"errors"
	"fmt"
	"github.com/mercari/tortoise/pkg/annotation"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
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

	d, err := ClientService.GetDeploymentOnTortoise(context.Background(), r)
	if err != nil {
		tortoiselog.Error(err, "failed to get deployment")
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

//+kubebuilder:webhook:path=/validate-autoscaling-mercari-com-v1alpha1-tortoise,mutating=false,failurePolicy=fail,sideEffects=None,groups=autoscaling.mercari.com,resources=tortoises,verbs=create;update,versions=v1alpha1,name=vtortoise.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Tortoise{}

func validateTortoise(t *Tortoise) error {
	fieldPath := field.NewPath("spec")

	if t.Spec.TargetRefs.DeploymentName == "" {
		return fmt.Errorf("%s: shouldn't be empty", fieldPath.Child("targetRefs", "deploymentName"))
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
func (r *Tortoise) ValidateCreate() error {
	ctx := context.Background()
	tortoiselog.Info("validate create", "name", r.Name)
	if err := validateTortoise(r); err != nil {
		return err
	}

	fieldPath := field.NewPath("spec")

	d, err := ClientService.GetDeploymentOnTortoise(ctx, r)
	if err != nil {
		return fmt.Errorf("failed to get the deployment defined in %s: %w", fieldPath.Child("targetRefs", "deploymentName"), err)
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
		return fmt.Errorf("%s: tortoise should have the policies for all containers defined in the deployment, but, it doesn't have the policy for the container(s) %v", fieldPath.Child("resourcePolicy"), noPolicyContainers)
	}
	uselessPolicies := policies.Difference(containers)
	if uselessPolicies.Len() != 0 {
		return fmt.Errorf("%s: tortoise should not have the policies for the container(s) which isn't defined in the deployment, but, it have the policy for the container(s) %v", fieldPath.Child("resourcePolicy"), uselessPolicies)
	}

	hpa, err := ClientService.GetHPAFromUser(ctx, r)
	if err != nil {
		// Check if HPA really exists or not.
		return fmt.Errorf("failed to get the horizontal pod autoscaler defined in %s: %w", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"), err)
	}
	if hpa != nil {
		for _, c := range containers.List() {
			err = validateHPAAnnotations(hpa, c)
			if err != nil {
				return fmt.Errorf("the horizontal pod autoscaler defined in %s is invalid: %w", fieldPath.Child("targetRefs", "horizontalPodAutoscalerName"), err)
			}
		}
	}

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
// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
func (r *Tortoise) ValidateDelete() error {
	tortoiselog.Info("validate delete", "name", r.Name)
	return nil
}

func externalMetricNameFromAnnotation(hpa *v2.HorizontalPodAutoscaler, containerName string, k corev1.ResourceName) (string, error) {
	var prefix string
	switch k {
	case corev1.ResourceCPU:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedCPUExternalMetricNamePrefixAnnotation]
	case corev1.ResourceMemory:
		prefix = hpa.GetAnnotations()[annotation.HPAContainerBasedMemoryExternalMetricNamePrefixAnnotation]
	default:
		return "", fmt.Errorf("non supported resource type: %s", k)
	}
	return prefix + containerName, nil
}

func validateHPAAnnotations(hpa *v2.HorizontalPodAutoscaler, containerName string) error {
	externalMetrics := sets.NewString()
	for _, m := range hpa.Spec.Metrics {
		if m.Type != v2.ExternalMetricSourceType {
			continue
		}

		if m.External == nil {
			// shouldn't reach here
			klog.ErrorS(nil, "invalid external metric on HPA", klog.KObj(hpa))
			continue
		}

		externalMetrics.Insert(m.External.Metric.Name)
	}

	resourceNames := []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
	for _, rn := range resourceNames {
		externalMetricName, err := externalMetricNameFromAnnotation(hpa, containerName, rn)
		if err != nil {
			return err
		}

		if !externalMetrics.Has(externalMetricName) {
			return fmt.Errorf("HPA doesn't have the external metrics which is defined in the annotations. (The annotation wants an external metric named %s)", externalMetricName)
		}
	}

	return nil
}
