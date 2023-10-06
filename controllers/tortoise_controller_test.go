package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/mercari/tortoise/api/v1beta1"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/recommender"
	"github.com/mercari/tortoise/pkg/tortoise"
	"github.com/mercari/tortoise/pkg/vpa"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newResource(path string) resources {
	tortoisePath := fmt.Sprintf("%s/tortoise.yaml", path)
	hpaPath := fmt.Sprintf("%s/hpa.yaml", path)
	deploymentPath := fmt.Sprintf("%s/deployment.yaml", path)
	updaterVPAPath := fmt.Sprintf("%s/vpa-Updater.yaml", path)
	monitorVPAPath := fmt.Sprintf("%s/vpa-Monitor.yaml", path)

	y, err := os.ReadFile(tortoisePath)
	Expect(err).NotTo(HaveOccurred())
	tortoise := &v1beta1.Tortoise{}
	err = yaml.Unmarshal(y, tortoise)
	Expect(err).NotTo(HaveOccurred())

	y, err = os.ReadFile(updaterVPAPath)
	Expect(err).NotTo(HaveOccurred())
	vpa := &autoscalingv1.VerticalPodAutoscaler{}
	err = yaml.Unmarshal(y, vpa)
	Expect(err).NotTo(HaveOccurred())

	y, err = os.ReadFile(monitorVPAPath)
	Expect(err).NotTo(HaveOccurred())
	vpa2 := &autoscalingv1.VerticalPodAutoscaler{}
	err = yaml.Unmarshal(y, vpa2)
	Expect(err).NotTo(HaveOccurred())

	var deploy *v1.Deployment
	y, err = os.ReadFile(deploymentPath)
	// maybe deployment file is not exist
	if err == nil {
		deploy = &v1.Deployment{}
		err = yaml.Unmarshal(y, deploy)
		Expect(err).NotTo(HaveOccurred())
	}

	var hpa *v2.HorizontalPodAutoscaler
	y, err = os.ReadFile(hpaPath)
	// maybe hpa file is not exist
	if err == nil {
		Expect(err).NotTo(HaveOccurred())
		hpa = &v2.HorizontalPodAutoscaler{}
		err = yaml.Unmarshal(y, hpa)
		Expect(err).NotTo(HaveOccurred())
	}

	return resources{
		tortoise:   tortoise,
		hpa:        hpa,
		deployment: deploy,
		vpa: map[v1beta1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
			v1beta1.VerticalPodAutoscalerRoleUpdater: vpa,
			v1beta1.VerticalPodAutoscalerRoleMonitor: vpa2,
		},
	}
}

func createDeploymentWithStatus(ctx context.Context, k8sClient client.Client, deploy *v1.Deployment) {
	err := k8sClient.Create(ctx, deploy.DeepCopy())
	Expect(err).NotTo(HaveOccurred())

	d := &v1.Deployment{}
	err = k8sClient.Get(ctx, client.ObjectKey{Namespace: deploy.Namespace, Name: deploy.Name}, d)
	Expect(err).NotTo(HaveOccurred())

	d.Status = deploy.Status
	err = k8sClient.Status().Update(ctx, d)
	Expect(err).NotTo(HaveOccurred())
}

func createVPAWithStatus(ctx context.Context, k8sClient client.Client, vpa *autoscalingv1.VerticalPodAutoscaler) {
	err := k8sClient.Create(ctx, vpa.DeepCopy())
	Expect(err).NotTo(HaveOccurred())

	v := &autoscalingv1.VerticalPodAutoscaler{}
	err = k8sClient.Get(ctx, client.ObjectKey{Namespace: vpa.Namespace, Name: vpa.Name}, v)
	Expect(err).NotTo(HaveOccurred())

	v.Status = vpa.Status
	err = k8sClient.Status().Update(ctx, v)
	Expect(err).NotTo(HaveOccurred())
}

func createTortoiseWithStatus(ctx context.Context, k8sClient client.Client, tortoise *v1beta1.Tortoise) {
	err := k8sClient.Create(ctx, tortoise.DeepCopy())
	Expect(err).NotTo(HaveOccurred())

	v := &v1beta1.Tortoise{}
	err = k8sClient.Get(ctx, client.ObjectKey{Namespace: tortoise.Namespace, Name: tortoise.Name}, v)
	Expect(err).NotTo(HaveOccurred())

	v.Status = tortoise.Status
	err = k8sClient.Status().Update(ctx, v)
	Expect(err).NotTo(HaveOccurred())
}

func initializeResourcesFromFiles(ctx context.Context, k8sClient client.Client, path string) resources {
	resource := newResource(path)
	if resource.hpa != nil {
		err := k8sClient.Create(ctx, resource.hpa)
		Expect(err).NotTo(HaveOccurred())
	}

	createDeploymentWithStatus(ctx, k8sClient, resource.deployment)
	createVPAWithStatus(ctx, k8sClient, resource.vpa[v1beta1.VerticalPodAutoscalerRoleUpdater])
	createVPAWithStatus(ctx, k8sClient, resource.vpa[v1beta1.VerticalPodAutoscalerRoleMonitor])
	createTortoiseWithStatus(ctx, k8sClient, resource.tortoise)

	return resource
}

func startController(ctx context.Context) func() {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).ShouldNot(HaveOccurred())

	tortoiseService, err := tortoise.New(mgr.GetClient(), record.NewFakeRecorder(10), 1, "Asia/Tokyo", 1000*time.Minute, "weekly")
	Expect(err).ShouldNot(HaveOccurred())
	cli, err := vpa.New(mgr.GetConfig(), record.NewFakeRecorder(10))
	Expect(err).ShouldNot(HaveOccurred())
	reconciler := &TortoiseReconciler{
		Scheme:             scheme,
		HpaService:         hpa.New(mgr.GetClient(), record.NewFakeRecorder(10), 0.95, 90),
		EventRecorder:      record.NewFakeRecorder(10),
		VpaService:         cli,
		DeploymentService:  deployment.New(mgr.GetClient()),
		TortoiseService:    tortoiseService,
		RecommenderService: recommender.New(24*30, 2.0, 0.5, 90, 3, 30, "10", "10Gi"),
	}
	err = reconciler.SetupWithManager(mgr)
	Expect(err).ShouldNot(HaveOccurred())

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		err := mgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	return cancel
}

var _ = Describe("Test TortoiseController", func() {
	ctx := context.Background()
	var stopFunc func()
	cleanUp := func() {
		err := deleteObj(ctx, &v1beta1.Tortoise{}, "mercari")
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
		}
		err = deleteObj(ctx, &v1.Deployment{}, "mercari-app")
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
		}
		err = deleteObj(ctx, &autoscalingv1.VerticalPodAutoscaler{}, "tortoise-updater-mercari")
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
		}
		err = deleteObj(ctx, &autoscalingv1.VerticalPodAutoscaler{}, "tortoise-monitor-mercari")
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
		}
		err = deleteObj(ctx, &v2.HorizontalPodAutoscaler{}, "tortoise-hpa-mercari")
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
		}
	}

	runTest := func(path string) {
		initializeResourcesFromFiles(ctx, k8sClient, filepath.Join(path, "before"))
		stopFunc = startController(ctx)
		tc := testCase{want: newResource(filepath.Join(path, "after"))}
		Eventually(func(g Gomega) {
			gotTortoise := &v1beta1.Tortoise{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
			g.Expect(err).ShouldNot(HaveOccurred())
			var gotHPA *v2.HorizontalPodAutoscaler
			if tc.want.hpa != nil {
				gotHPA = &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
			}
			gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
			g.Expect(err).ShouldNot(HaveOccurred())
			gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
			g.Expect(err).ShouldNot(HaveOccurred())

			err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA, vpa: map[v1beta1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
				v1beta1.VerticalPodAutoscalerRoleUpdater: gotUpdaterVPA,
				v1beta1.VerticalPodAutoscalerRoleMonitor: gotMonitorVPA,
			}})
			g.Expect(err).ShouldNot(HaveOccurred())

			cleanUp()
		}).Should(Succeed())
	}

	AfterEach(func() {
		suiteConfig, _ := GinkgoConfiguration()
		if CurrentSpecReport().Failed() && suiteConfig.FailFast {
			suiteFailed = true
		} else {
			cleanUp()
			for {
				// make sure all resources are deleted
				t := &v1beta1.Tortoise{}
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, t)
				if apierrors.IsNotFound(err) {
					break
				}
			}
		}

		stopFunc()
		time.Sleep(100 * time.Millisecond)
	})

	Context("reconcile for the single container Pod", func() {
		It("TortoisePhaseWorking", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-working"))
		})
		It("TortoisePhaseWorking (dryrun)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-dryrun"))
		})
		It("TortoisePhaseEmergency", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-emergency"))
		})
	})
	Context("reconcile for the multiple containers Pod", func() {
		It("TortoisePhaseWorking", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-working"))
		})
		It("TortoisePhaseWorking (include AutoscalingTypeOff)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-one-off"))
		})
		It("TortoisePhaseWorking (All AutoscalingTypeVertical)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-all-vertical"))
		})
		It("TortoisePhaseWorking (All AutoscalingTypeOff)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-all-off"))
		})
		It("TortoisePhaseEmergency", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-emergency"))
		})
	})
	Context("DeletionPolicy is handled correctly", func() {
		It("[DeletionPolicy = DeleteAll] delete HPA and VPAs when Tortoise is deleted", func() {
			resource := initializeResourcesFromFiles(ctx, k8sClient, "testdata/deletion-policy-all/before")
			stopFunc = startController(ctx)

			// wait the reconcile loop gives the finalizer to Tortoise
			time.Sleep(1 * time.Second)

			// delete Tortoise
			err := k8sClient.Delete(ctx, resource.tortoise)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func(g Gomega) {
				// make sure all resources are deleted
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, &v1beta1.Tortoise{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, &v2.HorizontalPodAutoscaler{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
			}).Should(Succeed())
		})
		It("[DeletionPolicy = NoDelete] do not delete HPA and VPAs when Tortoise is deleted", func() {
			resource := initializeResourcesFromFiles(ctx, k8sClient, "testdata/deletion-no-delete/before")
			stopFunc = startController(ctx)

			// delete Tortoise
			err := k8sClient.Delete(ctx, resource.tortoise)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func(g Gomega) {
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, &v2.HorizontalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, &v1beta1.Tortoise{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
			}).Should(Succeed())
		})
	})
})

type testCase struct {
	want resources
}

type resources struct {
	tortoise   *v1beta1.Tortoise
	deployment *v1.Deployment
	hpa        *v2.HorizontalPodAutoscaler
	vpa        map[v1beta1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler
}

func (t *testCase) compare(got resources) error {
	if d := cmp.Diff(t.want.tortoise, got.tortoise, cmpopts.IgnoreFields(v1beta1.Tortoise{}, "ObjectMeta"), cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
		return fmt.Errorf("unexpected tortoise: diff = %s", d)
	}
	if d := cmp.Diff(t.want.hpa, got.hpa, cmpopts.IgnoreFields(v2.HorizontalPodAutoscaler{}, "ObjectMeta"), cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
		return fmt.Errorf("unexpected hpa: diff = %s", d)
	}

	for k, vpa := range t.want.vpa {
		if d := cmp.Diff(vpa, got.vpa[k], cmpopts.IgnoreFields(autoscalingv1.VerticalPodAutoscaler{}, "ObjectMeta"), cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
			return fmt.Errorf("unexpected vpa[%s]: diff = %s", k, d)
		}
	}

	return nil
}

func deleteObj(ctx context.Context, deleteObj client.Object, name string) error {
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, deleteObj)
	if err != nil {
		return err
	}
	err = k8sClient.Delete(ctx, deleteObj)
	if err != nil {
		return err
	}
	return nil
}
