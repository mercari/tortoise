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

package main

import (
	"flag"
	"os"
	"time"

	"github.com/mercari/tortoise/pkg/tortoise"

	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/recommender"

	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/vpa"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	"github.com/mercari/tortoise/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(autoscalingv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	// Tortoise specific flags
	var rangeOfMinMaxReplicasRecommendationHours int
	var tTLHoursOfMinMaxReplicasRecommendation int
	var maxReplicasFactor float64
	var minReplicasFactor float64
	var replicaReductionFactor float64
	var upperTargetResourceUtilization int
	var minimumMinReplicas int
	var preferredReplicaNumUpperLimit int
	var maxCPUPerContainer string
	var maxMemoryPerContainer string
	var timeZone string
	var tortoiseUpdateInterval time.Duration
	flag.IntVar(&rangeOfMinMaxReplicasRecommendationHours, "range-of-min-max-replicas-recommendation-hours", 1, "the time (hours) range of minReplicas and maxReplicas recommendation (default: 1)")
	flag.IntVar(&tTLHoursOfMinMaxReplicasRecommendation, "ttl-hours-of-min-max-replicas-recommendation", 24*30, "the TTL of minReplicas and maxReplicas recommendation (default: 720 (=30 days))")
	flag.Float64Var(&maxReplicasFactor, "max-replicas-factor", 2.0, "the factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)")
	flag.Float64Var(&minReplicasFactor, "min-replicas-factor", 0.5, "the factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)")
	flag.Float64Var(&replicaReductionFactor, "replica-reduction-factor", 0.95, "the factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)")
	flag.IntVar(&upperTargetResourceUtilization, "upper-target-resource-utilization", 90, "the max target utilization that tortoise can give to the HPA (default: 90)")
	flag.IntVar(&minimumMinReplicas, "minimum-min-replicas", 3, "the minimum minReplicas that tortoise can give to the HPA (default: 3)")
	flag.IntVar(&preferredReplicaNumUpperLimit, "preferred-replicas-number-upper-limit", 30, "the replica number which the tortoise tries to keep the replica number less than. As said \"preferred\", the tortoise tries to keep the replicas number less than this, but it lets the replica number more than this when other \"required\" rule will be violated by this limit. (default: 30)")
	flag.StringVar(&maxCPUPerContainer, "maximum-cpu-cores", "10", "the maximum CPU cores that the tortoise can give to the container (default: 10)")
	flag.StringVar(&maxMemoryPerContainer, "maximum-memory-bytes", "10Gi", "the maximum memory bytes that the tortoise can give to the container (default: 10Gi)")
	flag.StringVar(&timeZone, "timezone", "Asia/Tokyo", "The timezone used to record time in tortoise objects (default: Asia/Tokyo)")
	flag.DurationVar(&tortoiseUpdateInterval, "tortoise-update-interval", 15*time.Second, "The interval of updating each tortoise (default: 15s)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "76c4d78a.mercari.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	tortoiseService, err := tortoise.New(mgr.GetClient(), rangeOfMinMaxReplicasRecommendationHours, timeZone, tortoiseUpdateInterval)
	if err != nil {
		setupLog.Error(err, "unable to start tortoise service")
		os.Exit(1)
	}

	vpaClient, err := vpa.New(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to start vpa client")
		os.Exit(1)
	}

	if err = (&controllers.TortoiseReconciler{
		Scheme:             mgr.GetScheme(),
		HpaClient:          hpa.New(mgr.GetClient(), replicaReductionFactor, upperTargetResourceUtilization),
		VpaClient:          vpaClient,
		DeploymentClient:   deployment.New(mgr.GetClient()),
		RecommenderService: recommender.New(tTLHoursOfMinMaxReplicasRecommendation, maxReplicasFactor, minReplicasFactor, upperTargetResourceUtilization, minimumMinReplicas, preferredReplicaNumUpperLimit, maxCPUPerContainer, maxMemoryPerContainer),
		TortoiseService:    tortoiseService,
		Interval:           30 * time.Second,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tortoise")
		os.Exit(1)
	}
	if err = (&autoscalingv1alpha1.Tortoise{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Tortoise")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
