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
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

	logger.Info("Reconciling ScheduledScaling", "name", scheduledScaling.Name, "namespace", scheduledScaling.Namespace, "type", scheduledScaling.Spec.Schedule.Type)

	// Validate the schedule configuration
	if err := scheduledScaling.Spec.Schedule.Validate(); err != nil {
		logger.Error(err, "Invalid schedule configuration")
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "InvalidSchedule", err.Error())
	}

	// Handle scheduling based on type
	switch scheduledScaling.Spec.Schedule.Type {
	case autoscalingv1alpha1.ScheduleTypeTime:
		return r.handleTimeBasedScheduling(ctx, scheduledScaling)
	case autoscalingv1alpha1.ScheduleTypeCron:
		return r.handleCronBasedScheduling(ctx, scheduledScaling)
	default:
		err := fmt.Errorf("unsupported schedule type: %s", scheduledScaling.Spec.Schedule.Type)
		logger.Error(err, "Invalid schedule type")
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "InvalidScheduleType", err.Error())
	}
}

// handleTimeBasedScheduling handles scheduling based on specific start/end times
func (r *ScheduledScalingReconciler) handleTimeBasedScheduling(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Parse the schedule times (already validated by Validate())
	startTime, _ := time.Parse(time.RFC3339, scheduledScaling.Spec.Schedule.StartAt)
	finishTime, _ := time.Parse(time.RFC3339, scheduledScaling.Spec.Schedule.FinishAt)

	now := time.Now()
	var newPhase autoscalingv1alpha1.ScheduledScalingPhase
	var reason, message string

	// Determine the current phase based on time
	if now.Before(startTime) {
		// Calculate time until start
		timeUntilStart := startTime.Sub(now)
		// Set status to Pending if not already set
		if scheduledScaling.Status.Phase != autoscalingv1alpha1.ScheduledScalingPhasePending {
			return ctrl.Result{RequeueAfter: timeUntilStart}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhasePending, "Waiting", fmt.Sprintf("Waiting for scheduled scaling to begin at %s", startTime.Format(time.RFC3339)))
		}
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

		// Set status to Active if not already set
		if scheduledScaling.Status.Phase != autoscalingv1alpha1.ScheduledScalingPhaseActive {
			// Update time-based status with formatted times
			if err := r.updateTimeBasedStatus(ctx, scheduledScaling, startTime, finishTime); err != nil {
				logger.Error(err, "Failed to update time-based status")
			}
			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseActive, "Active", fmt.Sprintf("Scheduled scaling is active until %s", finishTime.Format(time.RFC3339)))
		}

		// Calculate time until finish
		timeUntilFinish := finishTime.Sub(now)
		return ctrl.Result{RequeueAfter: timeUntilFinish}, nil
	}

	// Update status if phase changed (this handles Completed phase)
	if scheduledScaling.Status.Phase != newPhase {
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, newPhase, reason, message)
	}

	return ctrl.Result{}, nil
}

// handleCronBasedScheduling handles scheduling based on cron expressions
func (r *ScheduledScalingReconciler) handleCronBasedScheduling(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	now := time.Now()

	// Check if we're currently in an active scaling window
	isActive, endTime, err := scheduledScaling.Spec.Schedule.IsCurrentlyActive(now)
	if err != nil {
		logger.Error(err, "Failed to check if schedule is currently active")
		return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "ScheduleCheckFailed", err.Error())
	}

	if isActive {
		// We're in an active scaling period
		// Calculate current window start time
		duration, _ := time.ParseDuration(scheduledScaling.Spec.Schedule.Duration)
		currentStart := endTime.Add(-duration)

		// Apply scheduled scaling
		if err := r.applyScheduledScaling(ctx, scheduledScaling); err != nil {
			logger.Error(err, "Failed to apply scheduled scaling")
			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "ScalingFailed", err.Error())
		}

		// Set status to Active if not already set
		if scheduledScaling.Status.Phase != autoscalingv1alpha1.ScheduledScalingPhaseActive {
			// Update cron status with current window information
			if err := r.updateCronStatus(ctx, scheduledScaling, &currentStart, &endTime, nil); err != nil {
				logger.Error(err, "Failed to update cron status")
			}
			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseActive, "Active", fmt.Sprintf("Cron-based scheduled scaling is active until %s", endTime.Format(time.RFC3339)))
		}

		// Calculate time until this window ends
		timeUntilEnd := endTime.Sub(now)
		// Add a small buffer to ensure we catch the transition
		requeueAfter := timeUntilEnd + time.Minute
		if requeueAfter < time.Minute {
			requeueAfter = time.Minute
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	} else {
		// We're not in an active scaling period
		// Apply normal scaling (restore original settings) if we were previously active
		if scheduledScaling.Status.Phase == autoscalingv1alpha1.ScheduledScalingPhaseActive {
			if err := r.applyNormalScaling(ctx, scheduledScaling); err != nil {
				logger.Error(err, "Failed to apply normal scaling")
				return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "RestoreFailed", err.Error())
			}
		}

		// Find the next scheduled window
		nextStart, nextEnd, err := scheduledScaling.Spec.Schedule.GetNextScheduleWindow(now)
		if err != nil {
			logger.Error(err, "Failed to calculate next schedule window")
			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhaseFailed, "NextScheduleFailed", err.Error())
		}

		// Set status to Pending if not already set
		if scheduledScaling.Status.Phase != autoscalingv1alpha1.ScheduledScalingPhasePending {
			message := fmt.Sprintf("Next scheduled scaling window: %s to %s (cron: %s)",
				nextStart.Format(time.RFC3339),
				nextEnd.Format(time.RFC3339),
				scheduledScaling.Spec.Schedule.CronExpression)

			// Update cron status with next window information
			if err := r.updateCronStatus(ctx, scheduledScaling, nil, nil, &nextStart); err != nil {
				logger.Error(err, "Failed to update cron status")
			}

			return ctrl.Result{}, r.updateStatus(ctx, scheduledScaling, autoscalingv1alpha1.ScheduledScalingPhasePending, "WaitingForNext", message)
		}

		// Calculate time until next start
		timeUntilNext := nextStart.Sub(now)
		// Add a small buffer but ensure reasonable polling interval
		requeueAfter := timeUntilNext
		if requeueAfter > 10*time.Minute {
			// For far future schedules, check every 10 minutes
			requeueAfter = 10 * time.Minute
		} else if requeueAfter < time.Minute {
			// For immediate schedules, check in 1 minute
			requeueAfter = time.Minute
		}

		logger.V(1).Info("Cron schedule waiting for next window",
			"nextStart", nextStart,
			"timeUntilNext", timeUntilNext,
			"requeueAfter", requeueAfter)

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
}

// applyScheduledScaling applies the scheduled scaling configuration to the target Tortoise
func (r *ScheduledScalingReconciler) applyScheduledScaling(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling) error {
	// Get the target Tortoise
	t := &autoscalingv1beta3.Tortoise{}
	key := types.NamespacedName{Namespace: scheduledScaling.Namespace, Name: scheduledScaling.Spec.TargetRefs.TortoiseName}
	if err := r.Get(ctx, key, t); err != nil {
		return fmt.Errorf("failed to get target tortoise: %w", err)
	}

	// Preserve existing HPA reference if present and not already specified in spec
	if t.Spec.TargetRefs.HorizontalPodAutoscalerName == nil && t.Status.Targets.HorizontalPodAutoscaler != "" {
		// If tortoise created an HPA but spec doesn't reference it explicitly,
		// add the reference to prevent HPA recreation during scheduled scaling
		t.Spec.TargetRefs.HorizontalPodAutoscalerName = &t.Status.Targets.HorizontalPodAutoscaler
	}

	const annOriginal = "autoscaling.mercari.com/scheduledscaling-original-spec"
	const annMinReplicas = "autoscaling.mercari.com/scheduledscaling-min-replicas"
	if t.Annotations == nil {
		t.Annotations = map[string]string{}
	}

	// Persist original spec if not already stored
	if _, exists := t.Annotations[annOriginal]; !exists {
		orig, err := json.Marshal(t.Spec)
		if err != nil {
			return fmt.Errorf("marshal original tortoise spec: %w", err)
		}
		t.Annotations[annOriginal] = string(orig)
	}

	// Desired resource minimums from strategy
	dCPU := scheduledScaling.Spec.Strategy.Static.MinAllocatedResources.CPU
	dMem := scheduledScaling.Spec.Strategy.Static.MinAllocatedResources.Memory
	var qCPU, qMem resource.Quantity
	var hasCPU, hasMem bool
	if dCPU != "" {
		v, err := resource.ParseQuantity(dCPU)
		if err != nil {
			return fmt.Errorf("invalid cpu quantity %q: %w", dCPU, err)
		}
		qCPU = v
		hasCPU = true
	}
	if dMem != "" {
		v, err := resource.ParseQuantity(dMem)
		if err != nil {
			return fmt.Errorf("invalid memory quantity %q: %w", dMem, err)
		}
		qMem = v
		hasMem = true
	}

	updated := false
	for i := range t.Spec.ResourcePolicy {
		pol := &t.Spec.ResourcePolicy[i]
		if pol.MinAllocatedResources == nil {
			pol.MinAllocatedResources = v1.ResourceList{}
		}
		if hasCPU {
			curr := pol.MinAllocatedResources[v1.ResourceCPU]
			if curr.Cmp(qCPU) < 0 {
				pol.MinAllocatedResources[v1.ResourceCPU] = qCPU
				updated = true
			}
		}
		if hasMem {
			curr := pol.MinAllocatedResources[v1.ResourceMemory]
			if curr.Cmp(qMem) < 0 {
				pol.MinAllocatedResources[v1.ResourceMemory] = qMem
				updated = true
			}
		}
	}

	if m := scheduledScaling.Spec.Strategy.Static.MinimumMinReplicas; m > 0 {
		// Store intent; future versions could wire this to Tortoise min replicas recommendation
		t.Annotations[annMinReplicas] = fmt.Sprintf("%d", m)
		updated = true
	}

	if !updated {
		log.FromContext(ctx).Info("Scheduled scaling made no changes to tortoise spec", "tortoise", t.Name)
		return nil
	}
	if err := r.Update(ctx, t); err != nil {
		return fmt.Errorf("update tortoise: %w", err)
	}
	return nil
}

// applyNormalScaling restores the normal scaling configuration
func (r *ScheduledScalingReconciler) applyNormalScaling(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling) error {
	// Fetch target tortoise
	t := &autoscalingv1beta3.Tortoise{}
	key := types.NamespacedName{Namespace: scheduledScaling.Namespace, Name: scheduledScaling.Spec.TargetRefs.TortoiseName}
	if err := r.Get(ctx, key, t); err != nil {
		return fmt.Errorf("failed to get target tortoise for restore: %w", err)
	}

	const annOriginal = "autoscaling.mercari.com/scheduledscaling-original-spec"
	const annMinReplicas = "autoscaling.mercari.com/scheduledscaling-min-replicas"
	if t.Annotations == nil {
		return nil
	}
	orig, ok := t.Annotations[annOriginal]
	if !ok || orig == "" {
		return nil
	}
	var spec autoscalingv1beta3.TortoiseSpec
	if err := json.Unmarshal([]byte(orig), &spec); err != nil {
		return fmt.Errorf("unmarshal original tortoise spec: %w", err)
	}

	// Preserve HPA reference if it was added during scheduled scaling to prevent HPA recreation
	if spec.TargetRefs.HorizontalPodAutoscalerName == nil && t.Spec.TargetRefs.HorizontalPodAutoscalerName != nil && t.Status.Targets.HorizontalPodAutoscaler != "" {
		// If original spec didn't have HPA reference but current spec does (added during scheduled scaling),
		// preserve it to avoid HPA recreation when restoring
		spec.TargetRefs.HorizontalPodAutoscalerName = t.Spec.TargetRefs.HorizontalPodAutoscalerName
	}

	t.Spec = spec
	delete(t.Annotations, annOriginal)
	delete(t.Annotations, annMinReplicas)
	if err := r.Update(ctx, t); err != nil {
		return fmt.Errorf("restore tortoise: %w", err)
	}
	return nil
}

// updateStatus updates the status of the ScheduledScaling resource
func (r *ScheduledScalingReconciler) updateStatus(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling, phase autoscalingv1alpha1.ScheduledScalingPhase, reason, message string) error {
	if scheduledScaling.Status.Phase != phase {
		scheduledScaling.Status.Phase = phase
		scheduledScaling.Status.Reason = reason
		scheduledScaling.Status.Message = message
		scheduledScaling.Status.LastTransitionTime = metav1.Now()

		// Update human-readable schedule description
		scheduledScaling.Status.HumanReadableSchedule = scheduledScaling.GetHumanReadableSchedule()

		if err := r.Status().Update(ctx, scheduledScaling); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}
	}

	return nil
}

// updateTimeBasedStatus updates the time-based status fields with formatted times
func (r *ScheduledScalingReconciler) updateTimeBasedStatus(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling, startTime, finishTime time.Time) error {
	// Parse the times and create metav1.Time objects
	startMetaTime := metav1.NewTime(startTime)
	finishMetaTime := metav1.NewTime(finishTime)

	// Update the status fields
	scheduledScaling.Status.CurrentWindowStart = &startMetaTime
	scheduledScaling.Status.CurrentWindowEnd = &finishMetaTime
	scheduledScaling.Status.FormattedStartTime = scheduledScaling.GetFormattedTime(&startMetaTime)
	scheduledScaling.Status.FormattedEndTime = scheduledScaling.GetFormattedTime(&finishMetaTime)

	// For time-based schedules, Next Start is not applicable
	scheduledScaling.Status.FormattedNextStartTime = "N/A"

	// Update human-readable schedule description
	scheduledScaling.Status.HumanReadableSchedule = scheduledScaling.GetHumanReadableSchedule()

	if err := r.Status().Update(ctx, scheduledScaling); err != nil {
		return fmt.Errorf("failed to update time-based status: %w", err)
	}

	return nil
}

// updateCronStatus updates the cron-specific status fields
func (r *ScheduledScalingReconciler) updateCronStatus(ctx context.Context, scheduledScaling *autoscalingv1alpha1.ScheduledScaling, currentStart, currentEnd, nextStart *time.Time) error {
	// Update current window times
	if currentStart != nil {
		startTime := metav1.NewTime(*currentStart)
		scheduledScaling.Status.CurrentWindowStart = &startTime
		scheduledScaling.Status.FormattedStartTime = scheduledScaling.GetFormattedTime(&startTime)
	}
	if currentEnd != nil {
		endTime := metav1.NewTime(*currentEnd)
		scheduledScaling.Status.CurrentWindowEnd = &endTime
		scheduledScaling.Status.FormattedEndTime = scheduledScaling.GetFormattedTime(&endTime)
	}
	if nextStart != nil {
		nextTime := metav1.NewTime(*nextStart)
		scheduledScaling.Status.NextWindowStart = &nextTime
		scheduledScaling.Status.FormattedNextStartTime = scheduledScaling.GetFormattedTime(&nextTime)
	}

	// Update human-readable schedule description
	scheduledScaling.Status.HumanReadableSchedule = scheduledScaling.GetHumanReadableSchedule()

	if err := r.Status().Update(ctx, scheduledScaling); err != nil {
		return fmt.Errorf("failed to update cron status: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScheduledScalingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.ScheduledScaling{}).
		Complete(r)
}
