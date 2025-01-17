package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/yaml"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/features"
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
	monitorVPAPath := fmt.Sprintf("%s/vpa-Monitor.yaml", path)

	var tortoise *v1beta3.Tortoise
	y, err := os.ReadFile(tortoisePath)
	if err == nil {
		tortoise = &v1beta3.Tortoise{}
		err = yaml.Unmarshal(y, tortoise)
		Expect(err).NotTo(HaveOccurred())
	}

	var vpa *autoscalingv1.VerticalPodAutoscaler
	y, err = os.ReadFile(monitorVPAPath)
	if err == nil {
		vpa = &autoscalingv1.VerticalPodAutoscaler{}
		err = yaml.Unmarshal(y, vpa)
		Expect(err).NotTo(HaveOccurred())
	}

	var deploy *v1.Deployment
	y, err = os.ReadFile(deploymentPath)
	if err == nil {
		deploy = &v1.Deployment{}
		err = yaml.Unmarshal(y, deploy)
		Expect(err).NotTo(HaveOccurred())
	}

	var hpa *v2.HorizontalPodAutoscaler
	y, err = os.ReadFile(hpaPath)
	if err == nil {
		hpa = &v2.HorizontalPodAutoscaler{}
		err = yaml.Unmarshal(y, hpa)
		Expect(err).NotTo(HaveOccurred())
	}

	return resources{
		tortoise:   tortoise,
		hpa:        hpa,
		deployment: deploy,
		vpa:        vpa,
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

func createTortoiseWithStatus(ctx context.Context, k8sClient client.Client, tortoise *v1beta3.Tortoise) {
	err := k8sClient.Create(ctx, tortoise.DeepCopy())
	Expect(err).NotTo(HaveOccurred())

	v := &v1beta3.Tortoise{}
	err = k8sClient.Get(ctx, client.ObjectKey{Namespace: tortoise.Namespace, Name: tortoise.Name}, v)
	Expect(err).NotTo(HaveOccurred())

	if tortoise.Annotations["skip-status-update"] == "true" {
		return
	}
	v.Status = tortoise.Status
	err = k8sClient.Status().Update(ctx, v)
	Expect(err).NotTo(HaveOccurred())
}

func createHPAWithStatus(ctx context.Context, k8sClient client.Client, hpa *v2.HorizontalPodAutoscaler) {
	err := k8sClient.Create(ctx, hpa.DeepCopy())
	Expect(err).NotTo(HaveOccurred())

	h := &v2.HorizontalPodAutoscaler{}
	err = k8sClient.Get(ctx, client.ObjectKey{Namespace: hpa.Namespace, Name: hpa.Name}, h)
	Expect(err).NotTo(HaveOccurred())

	if !reflect.DeepEqual(hpa.Status, v2.HorizontalPodAutoscalerStatus{}) {
		h.Status = hpa.Status
		err = k8sClient.Status().Update(ctx, h)
		Expect(err).NotTo(HaveOccurred())
	}
}

func initializeResourcesFromFiles(ctx context.Context, k8sClient client.Client, path string) resources {
	resource := newResource(path)
	if resource.hpa != nil {
		createHPAWithStatus(ctx, k8sClient, resource.hpa)
	}

	createDeploymentWithStatus(ctx, k8sClient, resource.deployment)
	if resource.vpa != nil {
		createVPAWithStatus(ctx, k8sClient, resource.vpa)
	}
	createTortoiseWithStatus(ctx, k8sClient, resource.tortoise)

	return resource
}

func writeToFile(path string, r any) error {
	y, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	y, err = removeUnnecessaryFields(y)
	if err != nil {
		return err
	}
	err = os.WriteFile(path, y, 0644)
	if err != nil {
		return err
	}

	return nil
}

func updateResourcesInTestCaseFile(path string, resource resources) error {
	err := writeToFile(filepath.Join(path, "tortoise.yaml"), resource.tortoise)
	if err != nil {
		return err
	}

	err = writeToFile(filepath.Join(path, "deployment.yaml"), removeUnnecessaryFieldsFromDeployment(resource.deployment))
	if err != nil {
		return err
	}

	err = writeToFile(filepath.Join(path, "vpa-Monitor.yaml"), resource.vpa)
	if err != nil {
		return err
	}

	if resource.hpa != nil {
		err = writeToFile(filepath.Join(path, "hpa.yaml"), resource.hpa)
		if err != nil {
			return err
		}
	}

	return nil
}

func removeUnnecessaryFieldsFromDeployment(deployment *v1.Deployment) *v1.Deployment {
	// remove all default values

	deployment.Spec.ProgressDeadlineSeconds = nil
	deployment.Spec.Replicas = nil
	deployment.Spec.RevisionHistoryLimit = nil
	deployment.Spec.Strategy = v1.DeploymentStrategy{}
	deployment.Spec.Template.ObjectMeta.CreationTimestamp = metav1.Time{}
	deployment.Spec.Template.Spec.DNSPolicy = ""
	deployment.Spec.Template.Spec.RestartPolicy = ""
	deployment.Spec.Template.Spec.SchedulerName = ""
	deployment.Spec.Template.Spec.SecurityContext = nil
	deployment.Spec.Template.Spec.TerminationGracePeriodSeconds = nil
	for i := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[i].ImagePullPolicy = ""
		deployment.Spec.Template.Spec.Containers[i].TerminationMessagePath = ""
		deployment.Spec.Template.Spec.Containers[i].TerminationMessagePolicy = ""
	}

	deployment.Status = v1.DeploymentStatus{}

	return deployment
}

func removeUnnecessaryFields(rawdata []byte) ([]byte, error) {
	data := make(map[string]interface{})
	err := yaml.Unmarshal(rawdata, &data)
	if err != nil {
		return nil, err
	}
	meta, ok := data["metadata"]
	if !ok {
		return nil, errors.New("no metadata")
	}
	typed, ok := meta.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("metadata is unexpected type: %T", meta)
	}

	delete(typed, "creationTimestamp")
	delete(typed, "managedFields")
	delete(typed, "resourceVersion")
	delete(typed, "uid")
	delete(typed, "generation")

	return yaml.Marshal(data)
}

func startController(ctx context.Context) func() {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Controller: config.Controller{
			SkipNameValidation: ptr.To(true),
		},
	})
	Expect(err).ShouldNot(HaveOccurred())

	// We only reconcile once.
	recorder := mgr.GetEventRecorderFor("tortoise-controller")
	tortoiseService, err := tortoise.New(mgr.GetClient(), recorder, 24, "Asia/Tokyo", 1000*time.Minute, "daily")
	Expect(err).ShouldNot(HaveOccurred())
	cli, err := vpa.New(mgr.GetConfig(), recorder)
	Expect(err).ShouldNot(HaveOccurred())
	hpaS, err := hpa.New(mgr.GetClient(), recorder, 0.95, 90, 25, time.Hour, 1000, 10000, 3, ".*-exclude-metric")
	Expect(err).ShouldNot(HaveOccurred())
	reconciler := &TortoiseReconciler{
		Scheme:             scheme,
		HpaService:         hpaS,
		EventRecorder:      record.NewFakeRecorder(10),
		VpaService:         cli,
		DeploymentService:  deployment.New(mgr.GetClient(), "100m", "100Mi", recorder),
		TortoiseService:    tortoiseService,
		RecommenderService: recommender.New(2.0, 0.5, 90, 40, 3, 30, "10m", "10Mi", map[string]string{"istio-proxy": "11m"}, map[string]string{"istio-proxy": "11Mi"}, "10", "10Gi", 10000, 0, 0, []features.FeatureFlag{features.VerticalScalingBasedOnPreferredMaxReplicas}, recorder),
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
		err := deleteObj(ctx, &v1beta3.Tortoise{}, "mercari")
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
		}
		err = deleteObj(ctx, &v1.Deployment{}, "mercari-app")
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

	generateTestCases := func(path string) {
		// wait for the reconciliation.
		time.Sleep(1 * time.Second)
		path = filepath.Join(path, "after")
		Eventually(func(g Gomega) {
			gotTortoise := &v1beta3.Tortoise{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
			g.Expect(err).ShouldNot(HaveOccurred())
			var gotHPA *v2.HorizontalPodAutoscaler
			_, err = os.Stat(path + "/hpa.yaml")
			if err == nil {
				// HPA should also be regenerated
				gotHPA = &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
			}
			gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
			g.Expect(err).ShouldNot(HaveOccurred())

			// get deployment
			gotDeployment := &v1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari-app"}, gotDeployment)
			g.Expect(err).ShouldNot(HaveOccurred())

			err = updateResourcesInTestCaseFile(path, resources{
				tortoise:   gotTortoise,
				hpa:        gotHPA,
				vpa:        gotMonitorVPA,
				deployment: gotDeployment,
			})
			g.Expect(err).ShouldNot(HaveOccurred())
		}).Should(Succeed())
	}

	checkWithWantedResources := func(path string) {
		// wait for the reconciliation.
		time.Sleep(1 * time.Second)
		tc := testCase{want: newResource(filepath.Join(path, "after"))}
		Eventually(func(g Gomega) {
			gotTortoise := &v1beta3.Tortoise{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
			g.Expect(err).ShouldNot(HaveOccurred())
			var gotHPA *v2.HorizontalPodAutoscaler
			if tc.want.hpa != nil {
				gotHPA = &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
			} else {
				// HPA should not exist.
				gotHPA = &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				Expect(apierrors.IsNotFound(err)).To(Equal(true))
				gotHPA = nil
			}
			gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
			g.Expect(err).ShouldNot(HaveOccurred())

			// get deployment
			gotDeployment := &v1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari-app"}, gotDeployment)
			g.Expect(err).ShouldNot(HaveOccurred())

			err = tc.compare(resources{
				tortoise:   gotTortoise,
				hpa:        gotHPA,
				vpa:        gotMonitorVPA,
				deployment: gotDeployment,
			})
			g.Expect(err).ShouldNot(HaveOccurred())
		}).Should(Succeed())
	}

	runTest := func(path string) {
		initializeResourcesFromFiles(ctx, k8sClient, filepath.Join(path, "before"))
		stopFunc = startController(ctx)
		if os.Getenv("UPDATE_TESTCASES") == "true" {
			generateTestCases(path)
		} else {
			checkWithWantedResources(path)
		}
		cleanUp()
	}

	AfterEach(func() {
		suiteConfig, _ := GinkgoConfiguration()
		if CurrentSpecReport().Failed() && suiteConfig.FailFast {
			suiteFailed = true
		} else {
			cleanUp()
			for {
				// make sure all resources are deleted
				t := &v1beta3.Tortoise{}
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
		It("TortoisePhaseBackToNormal", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-backtonormal"))
		})
		It("TortoisePhaseWorking", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-working"))
		})
		It("TortoisePhaseWorking (too big rescommendation)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-too-big"))
		})
		It("TortoisePhaseWorking (PartlyWorking)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-partly-working"))
		})
		It("TortoisePhaseWorking (GatheringData)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-gathering-data"))
		})
		It("TortoisePhaseInitializing", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-initializing"))
		})
		It("TortoisePhaseWorking (GatheringData is just finished)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-gathering-data-finished"))
		})
		It("TortoisePhaseWorking (dryrun)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-dryrun"))
		})
		It("TortoisePhaseWorking and HPA changed", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-hpa-changed"))
		})
		It("user just enabled TortoisePhaseEmergency", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-emergency-started"))
		})
		It("TortoisePhaseEmergency", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-single-container-pod-during-emergency"))
		})
	})
	Context("reconcile for the multiple containers Pod", func() {
		It("TortoisePhaseWorking", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-working"))
		})
		It("TortoisePhaseWorking (istio)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-istio-enabled-pod-working"))
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
		It("TortoisePhaseWorking (VPA suggestion too small)", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-suggested-too-small"))
		})
		It("user just enabled TortoisePhaseEmergency", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-emergency-started"))
		})
		It("TortoisePhaseEmergency", func() {
			runTest(filepath.Join("testdata", "reconcile-for-the-multiple-containers-pod-during-emergency"))
		})
	})
	Context("mutable AutoscalingPolicy", func() {
		It("Tortoise get Horizontal and create HPA", func() {
			runTest(filepath.Join("testdata", "mutable-autoscalingpolicy-no-hpa-and-add-horizontal"))
		})
		It("Tortoise get another Horizontal and modify the existing HPA", func() {
			runTest(filepath.Join("testdata", "mutable-autoscalingpolicy-add-another-horizontal"))
		})
		It("Horizontal is removed and remove the HPA created by tortoise", func() {
			runTest(filepath.Join("testdata", "mutable-autoscalingpolicy-remove-horizontal"))
		})
		It("Horizontal is removed and modify the existing HPA", func() {
			runTest(filepath.Join("testdata", "mutable-autoscalingpolicy-remove-horizontal-2"))
		})
	})
	Context("automatic switch to emergency mode", func() {
		It("HPA scalingactive condition false", func() {
			runTest(filepath.Join("testdata", "reconcile-automatic-emergency-mode-hpa-condition"))
		})
		It("HPA scalingactive no metrics", func() {
			runTest(filepath.Join("testdata", "reconcile-automatic-emergency-mode-hpa-no-metrics"))
		})
		It("Tortoise changes the status back to Working if it finds HPA is working fine now", func() {
			runTest(filepath.Join("testdata", "reconcile-automatic-emergency-mode-hpa-back-to-working"))
		})
	})
	Context("DeletionPolicy is handled correctly", func() {
		It("[DeletionPolicy = DeleteAll] delete HPA and VPA when Tortoise is deleted", func() {
			resource := initializeResourcesFromFiles(ctx, k8sClient, "testdata/deletion-policy-all/before")
			stopFunc = startController(ctx)

			// wait the reconcile loop gives the finalizer to Tortoise
			time.Sleep(1 * time.Second)

			// delete Tortoise
			err := k8sClient.Delete(ctx, resource.tortoise)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func(g Gomega) {
				// make sure all resources are deleted
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, &v1beta3.Tortoise{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, &v2.HorizontalPodAutoscaler{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
			}).Should(Succeed())
		})
		It("[DeletionPolicy = NoDelete] do not delete HPA and VPA when Tortoise is deleted", func() {
			resource := initializeResourcesFromFiles(ctx, k8sClient, "testdata/deletion-no-delete/before")
			stopFunc = startController(ctx)

			// delete Tortoise
			err := k8sClient.Delete(ctx, resource.tortoise)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func(g Gomega) {
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, &v2.HorizontalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, &v1beta3.Tortoise{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
			}).Should(Succeed())
		})
	})
})

type testCase struct {
	want resources
}

type resources struct {
	tortoise   *v1beta3.Tortoise
	deployment *v1.Deployment
	hpa        *v2.HorizontalPodAutoscaler
	vpa        *autoscalingv1.VerticalPodAutoscaler
}

func (t *testCase) compare(got resources) error {
	if d := cmp.Diff(t.want.tortoise, got.tortoise, cmpopts.IgnoreFields(v1beta3.Tortoise{}, "ObjectMeta")); d != "" {
		return fmt.Errorf("unexpected tortoise: diff = %s", d)
	}
	if d := cmp.Diff(t.want.hpa, got.hpa, cmpopts.IgnoreFields(v2.HorizontalPodAutoscaler{}, "ObjectMeta")); d != "" {
		return fmt.Errorf("unexpected hpa: diff = %s", d)
	}
	// Only restartedAt annotation could be modified by the reconciliation
	// We don't care about the value, but the existence of the annotation.
	if got.deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] != t.want.deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] {
		return fmt.Errorf("restartedAt annotation is not expected: whether each has restartedAt annotation: got = %v, want = %v", got.deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"], t.want.deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"])
	}

	if d := cmp.Diff(t.want.vpa, got.vpa, cmpopts.IgnoreFields(autoscalingv1.VerticalPodAutoscaler{}, "ObjectMeta")); d != "" {
		return fmt.Errorf("unexpected vpa: diff = %s", d)
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
