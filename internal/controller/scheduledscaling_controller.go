/*
Copyright 2024 The Tortoise Authors.

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

package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
)

// ScheduledScalingReconciler reconciles a ScheduledScaling object
type ScheduledScalingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=scheduledscalings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=scheduledscalings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=scheduledscalings/finalizers,verbs=update
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ScheduledScalingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ScheduledScaling instance
	scheduledScaling := &autoscalingv1alpha1.ScheduledScaling{}
	if err := r.Get(ctx, req.NamespacedName, scheduledScaling); err != nil {
		// Handle the case where the resource is not found
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling ScheduledScaling", "name", scheduledScaling.Name, "namespace", scheduledScaling.Namespace)

	// Parse the schedule times
	startTime, err := time.Parse(time.RFC3339, scheduledScaling.Spec.Schedule.StartAt)
	if err != nil {
		logger.Error(err, "Failed to parse start time", "startAt", scheduledScaling.Spec.Schedule.StartAt)
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "InvalidStartTime", err.Error())
	}

	finishTime, err := time.Parse(time.RFC3339, scheduledScaling.Spec.Schedule.FinishAt)
	if err != nil {
		logger.Error(err, "Failed to parse finish time", "finishAt", scheduledScaling.Spec.Schedule.FinishAt)
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "InvalidFinishTime", err.Error())
	}

	// Validate that finish time is after start time
	if finishTime.Before(startTime) || finishTime.Equal(startTime) {
		err := fmt.Errorf("finish time must be after start time")
		logger.Error(err, "Invalid schedule", "startAt", startTime, "finishAt", finishTime)
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "InvalidSchedule", err.Error())
	}

	now := time.Now()
	var newPhase autoscalingv1alpha1.ScheduledScalingPhase
	var reason, message string

	// Determine the current phase based on time
	if now.Before(startTime) {
		// Calculate time until start
		timeUntilStart := startTime.Sub(now)
		return ctrl.Result{RequeueAfter: timeUntilStart}, nil
	} else if now.After(finishTime) {
		// Apply normal scaling (restore original settings)
		if err := r.applyNormalScaling(ctx, scheduledScaling); err != nil {
			logger.Error(err, "Failed to apply normal scaling")
			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "RestoreFailed", err.Error())
		}
		newPhase = autoscalingv1alpha1.ScheduledScalingPhaseCompleted
		reason = "Completed"
		message = "Scheduled scaling period has ended"
	} else {
		// Currently in the scaling period
		// Apply scheduled scaling
		if err := r.applyScheduledScaling(ctx, scheduledScaling); err != nil {
			logger.Error(err, "Failed to apply scheduled scaling")
			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "ScalingFailed", err.Error())
		}

		// Calculate time until finish
		timeUntilFinish := finishTime.Sub(now)
		return ctrl.Result{RequeueAfter: timeUntilFinish}, nil
	}

	// Update status if phase changed
	if scheduledScaling.Status.Phase != newPhase {
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, newPhase, reason, message)
	}

	return ctrl.Result{}, nil
}

// applyScheduledScaling applies the scheduled scaling configuration to the target Tortoise
func (r *ScheduledScalingReconciler) applyScheduledScaling(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling) error {
	// Get the target Tortoise
	tortoise := &autoscalingv1beta3.Tortoise{}
	tortoiseKey := types.NamespacedName{
		Namespace: scheduledScaling.Namespace,
		Name:      scheduledScaling.Spec.TargetRefs.TortoiseName,
	}

	if err := r.Get(ctx, tortoiseKey, tortoise); err != nil {
		return fmt.Errorf("failed to get target tortoise: %w", err)
	}

	// Apply the scheduled scaling configuration
	// This is a simplified implementation - in practice, you might want to:
	// 1. Store the original configuration before applying changes
	// 2. Apply the new configuration
	// 3. Handle conflicts with other scaling policies

	// For now, we'll just log what we would do
	logger := log.FromContext(ctx)
	logger.Info("Applying scheduled scaling",
		"tortoise", tortoise.Name,
		"minReplicas", scheduledScaling.Spec.Strategy.Static.MinimumMinReplicas,
		"cpu", scheduledScaling.Spec.Strategy.Static.MinAllocatedResources.CPU,
		"memory", scheduledScaling.Spec.Strategy.Static.MinAllocatedResources.Memory)

	return nil
}

// applyNormalScaling restores the normal scaling configuration
func (r *ScheduledScalingReconciler) applyNormalScaling(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling) error {
	logger := log.FromContext(ctx)
	logger.Info("Restoring normal scaling configuration", "tortoise", scheduledScaling.Spec.TargetRefs.TortoiseName)

	// In practice, you would restore the original configuration here
	// This might involve:
	// 1. Retrieving the stored original configuration
	// 2. Applying it to the Tortoise
	// 3. Cleaning up any temporary resources

	return nil
}

// updateStatus updates the status of the ScheduledScaling resource
func (r *ScheduledScalingReconciler) updateStatus(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling, phase autoscalingv1alpha1.ScheduledScalingPhase, reason, message string) error {
	if scheduledScaling.Status.Phase != phase {
		scheduledScaling.Status.Phase = phase
		scheduledScaling.Status.Reason = reason
		scheduledScaling.Status.Message = message
		scheduledScaling.Status.LastTransitionTime = metav1.Now()

		if err := r.Status().Update(ctx, scheduledScaling); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScheduledScalingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.ScheduledScaling{}).
		Complete(r)
}
