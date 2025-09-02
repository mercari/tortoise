package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var scheduledscalinglog = logf.Log.WithName("scheduledscaling-resource")

func (r *ScheduledScaling) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-autoscaling-mercari-com-v1alpha1-scheduledscaling,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1,groups=autoscaling.mercari.com,resources=scheduledscalings,verbs=create;update,versions=v1alpha1,name=vscheduledscaling.kb.io

var _ webhook.Validator = &ScheduledScaling{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ScheduledScaling) ValidateCreate() (admission.Warnings, error) {
	scheduledscalinglog.Info("validate create", "name", r.Name)

	if err := r.validateSpec(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ScheduledScaling) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	scheduledscalinglog.Info("validate update", "name", r.Name)

	if err := r.validateSpec(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ScheduledScaling) ValidateDelete() (admission.Warnings, error) {
	scheduledscalinglog.Info("validate delete", "name", r.Name)

	// No validation needed for delete
	return nil, nil
}

// validateSpec validates the ScheduledScaling spec
func (r *ScheduledScaling) validateSpec() error {
	var allErrs field.ErrorList

	// Validate that at least one of minimumMinReplicas or minAllocatedResources is present
	if err := r.validateAtLeastOneScalingParameter(); err != nil {
		allErrs = append(allErrs, err)
	}

	// Validate schedule
	if err := r.Spec.Schedule.Validate(); err != nil {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "schedule"),
			r.Spec.Schedule,
			err.Error(),
		))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return fmt.Errorf("validation failed: %v", allErrs)
}

// validateAtLeastOneScalingParameter ensures that at least one of minimumMinReplicas, minAllocatedResources, or containerMinAllocatedResources is present
func (r *ScheduledScaling) validateAtLeastOneScalingParameter() *field.Error {
	static := r.Spec.Strategy.Static

	// Check if all scaling parameters are missing
	if static.MinimumMinReplicas == nil &&
		static.MinAllocatedResources == nil &&
		len(static.ContainerMinAllocatedResources) == 0 {
		return field.Invalid(
			field.NewPath("spec", "strategy", "static"),
			r.Spec.Strategy.Static,
			"at least one of 'minimumMinReplicas', 'minAllocatedResources', or 'containerMinAllocatedResources' must be specified",
		)
	}

	return nil
}

// GetObjectKind implements runtime.Object
func (r *ScheduledScaling) GetObjectKind() schema.ObjectKind {
	return &r.TypeMeta
}
