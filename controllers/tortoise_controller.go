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
	"reflect"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/metrics"
	"github.com/mercari/tortoise/pkg/recommender"
	tortoiseService "github.com/mercari/tortoise/pkg/tortoise"
	"github.com/mercari/tortoise/pkg/vpa"
)

// TortoiseReconciler reconciles a Tortoise object
type TortoiseReconciler struct {
	Scheme *runtime.Scheme

	Interval time.Duration

	HpaService         *hpa.Service
	VpaService         *vpa.Service
	DeploymentService  *deployment.Service
	TortoiseService    *tortoiseService.Service
	RecommenderService *recommender.Service
	EventRecorder      record.EventRecorder
}

var (
	// During the test, we want to use a fixed time.
	onlyTestNow *time.Time
)

//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch

//+kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers/status,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete

func (r *TortoiseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)
	now := time.Now()
	if onlyTestNow != nil {
		now = *onlyTestNow
	}
	logger.Info("the reconciliation is started", "tortoise", req.NamespacedName)

	tortoise, err := r.TortoiseService.GetTortoise(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Probably deleted already and finalizer is already removed.
			logger.Info("tortoise is not found", "tortoise", req.NamespacedName)
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

		metrics.RecordTortoise(tortoise, true)
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	oldTortoise := tortoise.DeepCopy()

	defer func() {
		if tortoise == nil {
			logger.Error(reterr, "get error during the reconciliation, but cannot record the event because tortoise object is nil", "tortoise", req.NamespacedName)
			return
		}

		if metrics.ShouldRerecordTortoise(oldTortoise, tortoise) {
			metrics.RecordTortoise(oldTortoise, true)
		}
		metrics.RecordTortoise(tortoise, false)

		tortoise = r.TortoiseService.RecordReconciliationFailure(tortoise, reterr, now)
		_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, false)
		if err != nil {
			logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		}
	}()

	reconcileNow, requeueAfter := r.TortoiseService.ShouldReconcileTortoiseNow(tortoise, now)
	if !reconcileNow {
		logger.Info("the reconciliation is skipped because this tortoise is recently updated", "tortoise", req.NamespacedName)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Previously, we had VPA called "updator vpa". We don't need it anymore so we disable it here if Tortoise still has.
	// This logic can be removed later.
	err = r.VpaService.DisableTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		logger.Error(err, "delete updater VPA created by tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// TODO: stop depending on deployment.
	// https://github.com/mercari/tortoise/issues/129
	//
	// Currently, we don't depend on the deployment on almost all cases,
	// but we need to get the number of replicas from it + we need to take resource requests of each container when initializing tortoises.
	// We should be able to eventually remove this dependency by using the number of replicas from scale subresource.
	dm, err := r.DeploymentService.GetDeploymentOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get deployment", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}
	currentReplicaNum := dm.Status.Replicas

	if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff || tortoise.Status.Conditions.ContainerResourceRequests == nil {
		// If the update mode is off, we have to update ContainerResourceRequests from the deployment directly
		// so that pods will get an original resource request.
		// If it's not off, ContainerResourceRequests should be updated in UpdateVPAFromTortoiseRecommendation in the last reconciliation.
		acr, err := r.DeploymentService.GetResourceRequests(dm)
		if err != nil {
			logger.Error(err, "failed to get resource requests in deployment", "tortoise", req.NamespacedName, "deployment", klog.KObj(dm))
			return ctrl.Result{}, err
		}
		tortoise.Status.Conditions.ContainerResourceRequests = acr
	}

	hpa, err := r.HpaService.GetHPAOnTortoiseSpec(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get HPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}
	tortoise = tortoiseService.UpdateTortoiseAutoscalingPolicyInStatus(tortoise, hpa, now)
	tortoise = r.TortoiseService.UpdateTortoisePhase(tortoise, now)
	if tortoise.Status.TortoisePhase == autoscalingv1beta3.TortoisePhaseInitializing {
		logger.Info("initializing tortoise", "tortoise", req.NamespacedName)
		// need to initialize HPA and VPA.
		if err := r.initializeVPAAndHPA(ctx, tortoise, currentReplicaNum, now); err != nil {
			return ctrl.Result{}, fmt.Errorf("initialize VPAs and HPA: %w", err)
		}

		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	// Make sure finalizer is added to tortoise.
	tortoise, err = r.TortoiseService.AddFinalizer(ctx, tortoise)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
	}

	tortoise, err = r.HpaService.UpdateHPASpecFromTortoiseAutoscalingPolicy(ctx, tortoise, hpa, currentReplicaNum, now)
	if err != nil {
		logger.Error(err, "update HPA spec from Tortoise autoscaling policy", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	monitorvpa, ready, err := r.VpaService.GetTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get tortoise VPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}
	if !ready {
		logger.Info("VPA created by tortoise isn't ready yet", "tortoise", req.NamespacedName)
		tortoise.Status.TortoisePhase = autoscalingv1beta3.TortoisePhaseInitializing
		_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
		if err != nil {
			logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	_, err = r.VpaService.UpdateVPAContainerResourcePolicy(ctx, tortoise, monitorvpa)
	if err != nil {
		logger.Error(err, "update VPA Container Resource Policy", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// VPA is ready, we mark all Vertical scaling resources as Running.
	tortoise = vpa.SetAllVerticalContainerResourcePhaseWorking(tortoise, now)

	logger.Info("VPA created by tortoise is ready, proceeding to generate the recommendation", "tortoise", req.NamespacedName)
	hpa, err = r.HpaService.GetHPAOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get HPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateContainerRecommendationFromVPA(tortoise, monitorvpa, now)

	tortoise, err = r.RecommenderService.UpdateRecommendations(ctx, tortoise, hpa, currentReplicaNum, now)
	if err != nil {
		logger.Error(err, "update recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if tortoise.Status.TortoisePhase == autoscalingv1beta3.TortoisePhaseGatheringData {
		logger.Info("tortoise is GatheringData phase; skip applying the recommendation to HPA or VPAs")
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	_, tortoise, err = r.HpaService.UpdateHPAFromTortoiseRecommendation(ctx, tortoise, now)
	if err != nil {
		logger.Error(err, "update HPA based on the recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise, err = r.TortoiseService.UpdateResourceRequest(ctx, tortoise, currentReplicaNum, now)
	if err != nil {
		logger.Error(err, "update VPA based on the recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if tortoise.Spec.UpdateMode != v1beta3.UpdateModeOff && !reflect.DeepEqual(oldTortoise.Status.Conditions.ContainerResourceRequests, tortoise.Status.Conditions.ContainerResourceRequests) {
		// The container resource requests are updated, so we need to update the Pods.
		err = r.DeploymentService.RolloutRestart(ctx, dm, tortoise, now)
		if err != nil {
			logger.Error(err, "failed to rollout restart", "tortoise", req.NamespacedName)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: r.Interval}, nil
}

func (r *TortoiseReconciler) deleteVPAAndHPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, now time.Time) error {
	if tortoise.Spec.DeletionPolicy == autoscalingv1beta3.DeletionPolicyNoDelete {
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

func (r *TortoiseReconciler) initializeVPAAndHPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, replicaNum int32, now time.Time) error {
	// need to initialize HPA and VPA.
	tortoise, err := r.HpaService.InitializeHPA(ctx, tortoise, replicaNum, now)
	if err != nil {
		return err
	}

	_, tortoise, err = r.VpaService.CreateTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		return fmt.Errorf("create tortoise monitor VPA: %w", err)
	}
	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
	if err != nil {
		return fmt.Errorf("update Tortoise status: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TortoiseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1beta3.Tortoise{}).
		Complete(r)
}
