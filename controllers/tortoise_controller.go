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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/mercari/tortoise/api/v1beta3"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
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

	// TODO: the following fields should be removed after we stop depending on deployment.
	// IstioSidecarProxyDefaultCPU is the default CPU resource request of the istio sidecar proxy
	IstioSidecarProxyDefaultCPU string
	// IstioSidecarProxyDefaultMemory is the default Memory resource request of the istio sidecar proxy
	IstioSidecarProxyDefaultMemory string
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
		logger.V(4).Info("the reconciliation is skipped because this tortoise is recently updated", "tortoise", req.NamespacedName)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
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

	if tortoise.Status.TortoisePhase == autoscalingv1beta3.TortoisePhaseInitializing ||
		tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation == nil /* only the integration test */ {
		// Put the current resource requests into tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation.
		// Once we stop depending on deployments, we should put this logic in initializeTortoise.

		tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation = nil // reset
		for _, c := range dm.Spec.Template.Spec.Containers {
			rcr := v1beta3.RecommendedContainerResources{
				ContainerName:       c.Name,
				RecommendedResource: corev1.ResourceList{},
			}
			for name, r := range c.Resources.Requests {
				rcr.RecommendedResource[name] = r
			}
			tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation = append(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation, rcr)
		}

		if dm.Spec.Template.Annotations != nil {
			if v, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarInjectionAnnotation]; ok && v == "true" {
				// Istio sidecar injection is enabled.
				// Because the istio container spec is not in the deployment spec, we need to get it from the deployment's annotation.

				cpuReq, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarProxyCPUAnnotation]
				if !ok {
					cpuReq = r.IstioSidecarProxyDefaultCPU
				}
				cpu, err := resource.ParseQuantity(cpuReq)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("parse CPU request of istio sidecar: %w", err)
				}

				memoryReq, ok := dm.Spec.Template.Annotations[annotation.IstioSidecarProxyMemoryAnnotation]
				if !ok {
					memoryReq = r.IstioSidecarProxyDefaultMemory
				}
				memory, err := resource.ParseQuantity(memoryReq)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("parse Memory request of istio sidecar: %w", err)
				}
				// If the deployment has the sidecar injection annotation, the Pods will have the sidecar container in addition.
				tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation = append(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation, v1beta3.RecommendedContainerResources{
					ContainerName: "istio-proxy",
					RecommendedResource: corev1.ResourceList{
						corev1.ResourceCPU:    cpu,
						corev1.ResourceMemory: memory,
					},
				})
			}
		}
	}
	containerNames := deployment.GetContainerNames(dm)

	// === Finish the part depending on deployment ===
	// From here, we shouldn't use `dm` anymore.

	hpa, err := r.HpaService.GetHPAOnTortoiseSpec(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get HPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}
	tortoise = tortoiseService.UpdateTortoiseAutoscalingPolicyInStatus(tortoise, containerNames, hpa)
	tortoise = r.TortoiseService.UpdateTortoisePhase(tortoise, now)
	if tortoise.Status.TortoisePhase == autoscalingv1beta3.TortoisePhaseInitializing {
		logger.V(4).Info("initializing tortoise", "tortoise", req.NamespacedName)
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

	tortoise, err = r.HpaService.UpdateHPASpecFromTortoiseAutoscalingPolicy(ctx, tortoise, currentReplicaNum, now)
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
		logger.V(4).Info("VPA created by tortoise isn't ready yet", "tortoise", req.NamespacedName)
		tortoise.Status.TortoisePhase = autoscalingv1beta3.TortoisePhaseInitializing
		_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
		if err != nil {
			logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	// VPA is ready, we mark all Vertical scaling resources as Running.
	tortoise = vpa.SetAllVerticalContainerResourcePhaseWorking(tortoise, now)

	logger.V(4).Info("VPA created by tortoise is ready, proceeding to generate the recommendation", "tortoise", req.NamespacedName)
	hpa, err = r.HpaService.GetHPAOnTortoise(ctx, tortoise)
	if err != nil {
		logger.Error(err, "failed to get HPA", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	tortoise = r.TortoiseService.UpdateUpperRecommendation(tortoise, monitorvpa)

	tortoise, err = r.RecommenderService.UpdateRecommendations(ctx, tortoise, hpa, currentReplicaNum, now)
	if err != nil {
		logger.Error(err, "update recommendation in tortoise", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(tortoise, corev1.EventTypeNormal, "RecommendationUpdated", "The recommendation on Tortoise status is updated")

	if tortoise.Status.TortoisePhase == autoscalingv1beta3.TortoisePhaseGatheringData {
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

	_, err = r.TortoiseService.UpdateTortoiseStatus(ctx, tortoise, now, true)
	if err != nil {
		logger.Error(err, "update Tortoise status", "tortoise", req.NamespacedName)
		return ctrl.Result{}, err
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
	_, tortoise, err = r.VpaService.CreateTortoiseUpdaterVPA(ctx, tortoise)
	if err != nil {
		return fmt.Errorf("create tortoise updater VPA: %w", err)
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
