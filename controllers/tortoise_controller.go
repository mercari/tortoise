/*
Copyright 2023.

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

package controllers

import (
	"context"
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

	HpaClient          *hpa.Client
	VpaClient          *vpa.Client
	DeploymentClient   *deployment.Client
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

	dm, err := r.DeploymentClient.GetDeploymentOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get deployment", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateTortoisePhase(tortoise)
	if tortoise.Status.TortoisePhase == autoscalingv1alpha1.TortoisePhaseInitializing {
		// need to initialize HPA and VPA.
		if err := r.initializeVPAAndHPA(ctx, tortoise, dm); err != nil {
			return ctrl.Result{}, err
		}

		// VPA and HPA are just created, and they won't start working soon.
		// So, return here and wait a few min for them to start to work.
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}

	vpa, err := r.VpaClient.GetTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get tortoise VPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	hpa, err := r.HpaClient.GetHPAOnTortoise(ctx, tortoise)
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

	_, tortoise, err = r.HpaClient.UpdateHPAFromTortoiseRecommendation(ctx, tortoise, now)
	if err != nil {
		logger.Error(err, "update HPA based on the recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	_, err = r.VpaClient.UpdateVPAFromTortoiseRecommendation(ctx, tortoise)
	if err != nil {
		logger.Error(err, "update VPA based on the recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Interval}, nil
}

func (r *TortoiseReconciler) initializeVPAAndHPA(ctx context.Context, tortoise *autoscalingv1alpha1.Tortoise, dm *v1.Deployment) error {
	var err error
	// need to initialize HPA and VPA.
	tortoise, err = r.HpaClient.CreateHPAOnTortoise(ctx, tortoise, dm)
	if err != nil {
		return err
	}
	tortoise, err = r.VpaClient.CreateTortoiseMonitorVPA(ctx, tortoise)
	if err != nil {
		return err
	}
	tortoise, err = r.VpaClient.CreateTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		return err
	}
	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise)
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
