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

package main

import (
	"flag"
	"os"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	autoscalingv2 "github.com/mercari/tortoise/api/autoscaling/v2"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/controllers"
	"github.com/mercari/tortoise/pkg/config"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/recommender"
	"github.com/mercari/tortoise/pkg/tortoise"
	"github.com/mercari/tortoise/pkg/vpa"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(autoscalingv1beta3.AddToScheme(scheme))
	utilruntime.Must(autoscalingv1beta3.AddToScheme(scheme))
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
	var configPath string
	var logLevel string
	flag.StringVar(&configPath, "config", "", "The path to the config file.")
	flag.StringVar(&logLevel, "log-level", "DEBUG", "The log level: DEBUG, INFO, or ERROR")
	flag.Parse()

	level, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		setupLog.Error(err, "failed to parse log level, valid values are DEBUG, INFO, or ERROR")
		os.Exit(1)
	}
	zapconfig := zap.NewProductionConfig()
	zapconfig.Level = zap.NewAtomicLevelAt(level)
	zapconfig.DisableStacktrace = true
	zapconfig.Sampling = nil
	zapconfig.OutputPaths = []string{"stdout"}
	zapconfig.ErrorOutputPaths = []string{"stderr"}
	l, err := zapconfig.Build()
	if err != nil {
		setupLog.Error(err, "failed to create logger")
		os.Exit(1)
	}

	ctrl.SetLogger(zapr.NewLogger(l))

	config, err := config.ParseConfig(configPath)
	if err != nil {
		setupLog.Error(err, "failed to load config")
		os.Exit(1)
	}

	setupLog.Info("config", "config", *config)

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
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	eventRecorder := mgr.GetEventRecorderFor("tortoise-controller")
	tortoiseService, err := tortoise.New(mgr.GetClient(), eventRecorder, config.RangeOfMinMaxReplicasRecommendationHours, config.TimeZone, config.TortoiseUpdateInterval, config.GatheringDataPeriodType)
	if err != nil {
		setupLog.Error(err, "unable to start tortoise service")
		os.Exit(1)
	}

	vpaClient, err := vpa.New(mgr.GetConfig(), eventRecorder)
	if err != nil {
		setupLog.Error(err, "unable to start vpa client")
		os.Exit(1)
	}

	hpaService := hpa.New(mgr.GetClient(), eventRecorder, config.ReplicaReductionFactor, config.UpperTargetResourceUtilization, config.TortoiseHPATargetUtilizationMaxIncrease, config.TortoiseHPATargetUtilizationUpdateInterval)

	if err = (&controllers.TortoiseReconciler{
		Scheme:             mgr.GetScheme(),
		HpaService:         hpaService,
		VpaService:         vpaClient,
		DeploymentService:  deployment.New(mgr.GetClient(), config.IstioSidecarProxyDefaultCPU, config.IstioSidecarProxyDefaultMemory),
		RecommenderService: recommender.New(config.TTLHoursOfMinMaxReplicasRecommendation, config.MaxReplicasFactor, config.MinReplicasFactor, config.UpperTargetResourceUtilization, config.MinimumMinReplicas, config.PreferredReplicaNumUpperLimit, config.MaximumCPUCores, config.MaximumMemoryBytes, eventRecorder),
		TortoiseService:    tortoiseService,
		Interval:           config.TortoiseUpdateInterval,
		EventRecorder:      eventRecorder,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tortoise")
		os.Exit(1)
	}
	if err = (&autoscalingv1beta3.Tortoise{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Tortoise")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	hpaWebhook := autoscalingv2.New(tortoiseService, hpaService)

	if err = ctrl.NewWebhookManagedBy(mgr).
		WithDefaulter(hpaWebhook).
		WithValidator(hpaWebhook).
		For(&v2.HorizontalPodAutoscaler{}).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "HorizontalPodAutoscaler")
		os.Exit(1)
	}

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
