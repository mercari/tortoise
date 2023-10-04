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

package controllers

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1beta1 "github.com/mercari/tortoise/api/v1beta1"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/recommender"
	"github.com/mercari/tortoise/pkg/tortoise"
	"github.com/mercari/tortoise/pkg/vpa"
)

// TortoiseReconciler reconciles a Tortoise object
type TortoiseReconciler struct {
	Scheme *runtime.Scheme

	Interval time.Duration

	HpaService         *hpa.Service
	VpaService         *vpa.Service
	DeploymentService  *deployment.Service
	TortoiseService    *tortoise.Service
	RecommenderService *recommender.Service
	EventRecorder      record.EventRecorder
}

//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch

//+kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete

func (r *TortoiseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)
	now := time.Now()
	logger.V(4).Info("the reconciliation is started", "tortoise", req.NamespacedName)

	tortoise, err := r.TortoiseService.GetTortoise(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Probably deleted already and finalizer is already removed.
			logger.V(4).Info("tortoise is not found", "tortoise", req.NamespacedName)
			return ctrl.Result{}, nil
		}

		logger.Error(err, "failed to get tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !tortoise.ObjectMeta.DeletionTimestamp.IsZero() {
		// Tortoise is deleted by user and waiting for finalizer.
		logger.Info("tortoise is deleted", "tortoise", req.NamespacedName)
		if err := r.deleteVPAAndHPA(ctx, tortoise, now); err != nil {
			return ctrl.Result{}, fmt.Errorf("delete VPAs and HPA: %w", err)
		}
		if err := r.TortoiseService.RemoveFinalizer(ctx, tortoise); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	// tortoise is not deleted. Make sure finalizer is added to tortoise.
	if err := r.TortoiseService.AddFinalizer(ctx, tortoise); err != nil {
		return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
	}

	reconcileNow, requeueAfter := r.TortoiseService.ShouldReconcileTortoiseNow(tortoise, now)
	if !reconcileNow {
		logger.V(4).Info("the reconciliation is skipped because this tortoise is recently updated", "tortoise", req.NamespacedName)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Need to register defer after `ShouldReconcileTortoiseNow` because it updates the tortoise status in it.
	defer func() {
		tortoise = r.TortoiseService.RecordReconciliationFailure(tortoise, reterr, now)
		_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now)
		if err != nil {
			logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		}
	}()

	dm, err := r.DeploymentService.GetDeploymentOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get deployment", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateTortoisePhase(tortoise, dm)
	if tortoise.Status.TortoisePhase == autoscalingv1beta1.TortoisePhaseInitializing {
		logger.V(4).Info("initializing tortoise", "tortoise", req.NamespacedName)
		// need to initialize HPA and VPA.
		if err := r.initializeVPAAndHPA(ctx, tortoise, dm, now); err != nil {
			return ctrl.Result{}, fmt.Errorf("initialize VPAs and HPA: %w", err)
		}

		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	vpa, ready, err := r.VpaService.GetTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get tortoise VPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}
	if !ready {
		logger.V(4).Info("VPA created by tortoise isn't ready yet", "tortoise", req.NamespacedName)
		tortoise.Status.TortoisePhase = autoscalingv1beta1.TortoisePhaseInitializing
		_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now)
		if err != nil {
			logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	logger.V(4).Info("VPA created by tortoise is ready, proceeding to generate the recommendation", "tortoise", req.NamespacedName)
	hpa, err := r.HpaService.GetHPAOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get HPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateUpperRecommendation(tortoise, vpa)

	tortoise, err = r.RecommenderService.UpdateRecommendations(ctx, tortoise, hpa, dm, now)
	if err != nil {
		logger.Error(err, "update recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(tortoise, corev1.EventTypeNormal, "RecommendationUpdated", "The recommendation on Tortoise status is updated")

	if tortoise.Status.TortoisePhase == autoscalingv1beta1.TortoisePhaseGatheringData {
		logger.V(4).Info("tortoise is GatheringData phase; skip applying the recommendation to HPA or VPAs")
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	_, tortoise, err = r.HpaService.UpdateHPAFromTortoiseRecommendation(ctx, tortoise, now)
	if err != nil {
		logger.Error(err, "update HPA based on the recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	_, err = r.VpaService.UpdateVPAFromTortoiseRecommendation(ctx, tortoise)
	if err != nil {
		logger.Error(err, "update VPA based on the recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Interval}, nil
}

func (r *TortoiseReconciler) deleteVPAAndHPA(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise, now time.Time) error {
	if tortoise.Spec.DeletionPolicy == autoscalingv1beta1.DeletionPolicyNoDelete {
		// don't delete anything.
		return nil
	}

	var err error
	if tortoise.Spec.TargetRefs.HorizontalPodAutoscalerName == nil {
		// delete HPA created by tortoise
		err = r.HpaService.DeleteHPACreatedByTortoise(ctx, tortoise)
		if err != nil {
			return fmt.Errorf("delete HPA created by tortoise: %w", err)
		}
	}

	err = r.VpaService.DeleteTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		return fmt.Errorf("delete monitor VPA created by tortoise: %w", err)
	}
	err = r.VpaService.DeleteTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		return fmt.Errorf("delete updater VPA created by tortoise: %w", err)
	}
	return nil
}

func (r *TortoiseReconciler) initializeVPAAndHPA(ctx context.Context, tortoise *autoscalingv1beta1.Tortoise, dm *v1.Deployment, now time.Time) error {
	// need to initialize HPA and VPA.
	tortoise, err := r.HpaService.InitializeHPA(ctx, tortoise, dm)
	if err != nil {
		return err
	}

	_, tortoise, err = r.VpaService.CreateTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		return fmt.Errorf("create tortoise monitor VPA: %w", err)
	}
	_, tortoise, err = r.VpaService.CreateTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		return fmt.Errorf("create tortoise updater VPA: %w", err)
	}
	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now)
	if err != nil {
		return fmt.Errorf("update Tortoise status: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TortoiseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1beta1.Tortoise{}).
		Complete(r)
}
