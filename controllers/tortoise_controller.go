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

package controllers

import (
	"context"
	"fmt"
	v1 "k8s.io/api/apps/v1"
	"time"

	"github.com/mercari/tortoise/pkg/deployment"

	"github.com/mercari/tortoise/pkg/recommender"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/mercari/tortoise/pkg/tortoise"

	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/vpa"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
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
}

//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoscaling.mercari.com,resources=tortoises/finalizers,verbs=update

func (r *TortoiseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	now := time.Now()

	tortoise, err := r.TortoiseService.GetTortoise(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Probably deleted.
			logger.V(4).Info("tortoise is not found", "tortoise", req.NamespacedName)
			// TODO: delete VPA and HPA created by the Tortoise?
			return ctrl.Result{}, nil
		}

		logger.Error(err, "failed to get tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	reconcileNow, requeueAfter := r.TortoiseService.ShouldReconcileTortoiseNow(tortoise, now)
	if !reconcileNow {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	dm, err := r.DeploymentService.GetDeploymentOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get deployment", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateTortoisePhase(tortoise, dm)
	if tortoise.Status.TortoisePhase == autoscalingv1alpha1.TortoisePhaseInitializing {
		// need to initialize HPA and VPA.
		if err := r.initializeVPAAndHPA(ctx, tortoise, dm, now); err != nil {
			return ctrl.Result{}, fmt.Errorf("initialize VPAs and HPA: %w", err)
		}

		// VPA and HPA are just created, and they won't start working soon.
		// So, return here and wait a few min for them to start to work.
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}

	vpa, err := r.VpaService.GetTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get tortoise VPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	hpa, err := r.HpaService.GetHPAOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get HPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateUpperRecommendation(tortoise, vpa)

	tortoise, err = r.RecommenderService.UpdateRecommendations(tortoise, hpa, dm, now)
	if err != nil {
		logger.Error(err, "update recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
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

	return ctrl.Result{}, nil
}

func (r *TortoiseReconciler) initializeVPAAndHPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, dm *v1.Deployment, now time.Time) error {
	var err error
	// need to initialize HPA and VPA.
	_, tortoise, err = r.HpaService.CreateHPAOnTortoise(ctx, tortoise, dm)
	if err != nil {
		return err
	}
	_, tortoise, err = r.VpaService.CreateTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		return err
	}
	_, tortoise, err = r.VpaService.CreateTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		return err
	}
	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now)
	if err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TortoiseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.Tortoise{}).
		Complete(r)
}
