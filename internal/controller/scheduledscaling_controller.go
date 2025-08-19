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

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	tortoiseService "github.com/mercari/tortoise/pkg/tortoise"
)

// ScheduledScalingReconciler reconciles a ScheduledScaling object
type ScheduledScalingReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	TortoiseService *tortoiseService.Service
}

// +kubebuilder:rbac:groups=autoscaling.mercari.com,resources=scheduledscalings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.mercari.com,resources=scheduledscalings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.mercari.com,resources=scheduledscalings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ScheduledScaling object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ScheduledScalingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	now := time.Now()

	logger.Info("the reconciliation is started", "scheduledScaling", req.NamespacedName)

	// get scheduledScaling
	scheduledScaling := &autoscalingv1alpha1.ScheduledScaling{}
	err := r.Get(ctx, req.NamespacedName, scheduledScaling)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("scheduledScaling is not found", "scheduledScaling", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get scheduledScaling", "scheduledScaling", req.NamespacedName)
		return ctrl.Result{}, err
	}

	startAt := time.Time{}
	if scheduledScaling.Spec.Schedule.StartAt != nil {
		startAt, err = time.Parse(time.RFC3339, *scheduledScaling.Spec.Schedule.StartAt)
		if err != nil {
			logger.Error(err, "incorrect startAt format", "scheduledScaling", req.NamespacedName)
			return ctrl.Result{}, err
		}
	}

	if scheduledScaling.Spec.Schedule.FinishAt == nil {
		logger.Error(fmt.Errorf("finishAt is required"), "finishAt is not set", "scheduledScaling", req.NamespacedName)
		return ctrl.Result{}, fmt.Errorf("finishAt is required")
	}

	finishAt, err := time.Parse(time.RFC3339, *scheduledScaling.Spec.Schedule.FinishAt)
	if err != nil {
		logger.Error(err, "incorrect finishAt format", "scheduledScaling", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// if startAt empty or now is between startAt and finishAt,
	if startAt == (time.Time{}) || (now.After(startAt) && now.Before(finishAt)) {
		// if status inactive
		if scheduledScaling.Status.ScheduledScalingPhase == autoscalingv1alpha1.ScheduledScalingInactive {
			// get tortoise
			if scheduledScaling.Spec.TargetRefs.TortoiseName == nil {
				logger.Error(fmt.Errorf("tortoiseName is required"), "tortoiseName is not set", "scheduledScaling", req.NamespacedName)
				return ctrl.Result{}, fmt.Errorf("tortoiseName is required")
			}

			_, err := r.TortoiseService.GetTortoise(ctx, types.NamespacedName{Namespace: scheduledScaling.Namespace, Name: *scheduledScaling.Spec.TargetRefs.TortoiseName})
			if err != nil {
				if apierrors.IsNotFound(err) {
					// Probably deleted already and finalizer is already removed.
					logger.Info("tortoise is not found", "scheduledScaling", req.NamespacedName)
					return ctrl.Result{}, nil
				}

				logger.Error(err, "failed to get tortoise", "scheduledScaling", req.NamespacedName)
				return ctrl.Result{}, err
			}
			// TODO: sets ScheduledScaling recommender at Tortoise.spec.recommenders
		}
	}

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScheduledScalingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.ScheduledScaling{}).
		Complete(r)
}
