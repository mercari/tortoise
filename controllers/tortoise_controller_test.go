package controllers

import (
	"context"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/mercari/tortoise/api/v1alpha1"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/recommender"
	"github.com/mercari/tortoise/pkg/tortoise"
	"github.com/mercari/tortoise/pkg/utils"
	"github.com/mercari/tortoise/pkg/vpa"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"

	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"time"
)

var _ = Describe("Test TortoiseController", func() {
	ctx := context.Background()
	var stopFunc func()

	BeforeEach(func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ShouldNot(HaveOccurred())

		tortoiseService, err := tortoise.New(mgr.GetClient(), 1, "Asia/Tokyo", 1000*time.Minute)
		Expect(err).ShouldNot(HaveOccurred())
		cli, err := vpa.New(mgr.GetConfig())
		Expect(err).ShouldNot(HaveOccurred())
		reconciler := &TortoiseReconciler{
			Scheme:             scheme,
			HpaService:         hpa.New(mgr.GetClient(), 0.95, 90),
			VpaService:         cli,
			DeploymentService:  deployment.New(mgr.GetClient()),
			TortoiseService:    tortoiseService,
			RecommenderService: recommender.New(24*30, 2.0, 0.5, 90, 3, 30, "10", "10Gi"),
		}
		err = reconciler.SetupWithManager(mgr)
		Expect(err).ShouldNot(HaveOccurred())

		ctx, cancel := context.WithCancel(ctx)
		stopFunc = cancel
		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		stopFunc()
		time.Sleep(100 * time.Millisecond)
	})

	Context("reconcile for the single container Pod", func() {
		It("TortoisePhaseWorking", func() {
			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName:              "mercari-app",
							HorizontalPodAutoscalerName: pointer.String("hpa"),
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						SetTortoisePhase(v1alpha1.TortoisePhaseWorking).
						SetRecommendations(v1alpha1.Recommendations{
							Horizontal: &v1alpha1.HorizontalRecommendations{
								TargetUtilizations: []v1alpha1.HPATargetUtilizationRecommendationPerContainer{
									{
										ContainerName: "app",
										TargetUtilization: map[corev1.ResourceName]int32{
											corev1.ResourceCPU: 50, // will be updated.
										},
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   now.Weekday(),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   now.Weekday(),
										TimeZone:  now.Location().String(),
										Value:     3, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
							},
						}).
						AddCondition(v1alpha1.ContainerRecommendationFromVPA{
							ContainerName: "app",
							Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
								corev1.ResourceCPU:    {},
								corev1.ResourceMemory: {},
							},
							MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
								corev1.ResourceCPU:    {},
								corev1.ResourceMemory: {},
							},
						}).
						Build(),
					deployment: deploymentWithReplicaNum(10),
				},
			}

			err := tc.createResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// create the desired HPA from the created definition.
			wantHPA := tc.before.hpa.DeepCopy()
			wantHPA.Spec.MinReplicas = pointer.Int32(5)
			wantHPA.Spec.MaxReplicas = 20
			for i, m := range wantHPA.Spec.Metrics {
				if m.External != nil && m.External.Metric.Name == "datadogmetric@default:mercari-app-cpu-app" {
					wantHPA.Spec.Metrics[i].External.Target.Value = resourceQuantityPtr(resource.MustParse("75"))
				}
			}

			// create the desired VPA from the created definition.
			wantVPA := map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{}
			wantUpdater := tc.before.vpa[v1alpha1.VerticalPodAutoscalerRoleUpdater].DeepCopy()
			wantUpdater.Status.Recommendation = &autoscalingv1.RecommendedPodResources{}
			wantUpdater.Status.Recommendation.ContainerRecommendations = []autoscalingv1.RecommendedContainerResources{
				{
					ContainerName: "app",
					Target: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					LowerBound: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					UpperBound: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					UncappedTarget: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
				},
			}
			wantVPA[v1alpha1.VerticalPodAutoscalerRoleUpdater] = wantUpdater
			wantVPA[v1alpha1.VerticalPodAutoscalerRoleMonitor] = tc.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor].DeepCopy()

			want := resources{
				tortoise: utils.NewTortoiseBuilder().
					SetName("mercari").
					SetNamespace("default").
					SetTargetRefs(v1alpha1.TargetRefs{
						DeploymentName:              "mercari-app",
						HorizontalPodAutoscalerName: pointer.String("hpa"),
					}).
					AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
						ContainerName: "app",
						AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
							corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
						},
					}).
					SetTortoisePhase(v1alpha1.TortoisePhaseWorking).
					SetTargetsStatus(v1alpha1.TargetsStatus{
						HorizontalPodAutoscaler: "hpa",
						VerticalPodAutoscalers: []v1alpha1.TargetStatusVerticalPodAutoscaler{
							{Name: "tortoise-updater-mercari", Role: "Updater"},
							{Name: "tortoise-monitor-mercari", Role: "Monitor"},
						},
					}).
					SetRecommendations(v1alpha1.Recommendations{
						Vertical: &v1alpha1.VerticalRecommendations{
							ContainerResourceRecommendation: []v1alpha1.RecommendedContainerResources{
								{
									ContainerName: "app",
									RecommendedResource: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceCPU:    resource.MustParse("4"),
										corev1.ResourceMemory: resource.MustParse("3Gi"),
									},
								},
							},
						},
						Horizontal: &v1alpha1.HorizontalRecommendations{
							TargetUtilizations: []v1alpha1.HPATargetUtilizationRecommendationPerContainer{
								{
									ContainerName: "app",
									TargetUtilization: map[corev1.ResourceName]int32{
										corev1.ResourceCPU:    75,
										corev1.ResourceMemory: 90,
									},
								},
							},
							MaxReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   now.Weekday(),
									TimeZone:  now.Location().String(),
									Value:     20,
									UpdatedAt: metav1.NewTime(now),
								},
							},
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   now.Weekday(),
									TimeZone:  now.Location().String(),
									Value:     5,
									UpdatedAt: metav1.NewTime(now),
								},
							},
						},
					}).
					AddCondition(v1alpha1.ContainerRecommendationFromVPA{
						ContainerName: "app",
						Recommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("3"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("3Gi"),
							},
						},
						MaxRecommendation: map[corev1.ResourceName]v1alpha1.ResourceQuantity{
							corev1.ResourceCPU: {
								Quantity: resource.MustParse("3"),
							},
							corev1.ResourceMemory: {
								Quantity: resource.MustParse("3Gi"),
							},
						},
					}).
					Build(),
				hpa: wantHPA,
			}
			tc.want = want
			Eventually(func(g Gomega) {
				gotTortoise := &v1alpha1.Tortoise{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotHPA := &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "hpa"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
				g.Expect(err).ShouldNot(HaveOccurred())

				err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA})
				g.Expect(err).ShouldNot(HaveOccurred())
			}).Should(Succeed())
		})
		It("TortoisePhaseEmergency", func() {
			// TODO: add
		})
	})
})

type testCase struct {
	before resources
	want   resources
}

type resources struct {
	tortoise   *v1alpha1.Tortoise
	deployment *v1.Deployment
	hpa        *v2.HorizontalPodAutoscaler
	vpa        map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler
}

func (t *testCase) compare(got resources) error {
	if d := cmp.Diff(t.want.tortoise, got.tortoise, cmpopts.IgnoreFields(v1alpha1.Tortoise{}, "ObjectMeta"), cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
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

	if t.want.deployment != nil {
		if d := cmp.Diff(t.want.deployment, got.deployment, cmpopts.IgnoreFields(v1.Deployment{}, "ObjectMeta"), cmpopts.IgnoreTypes(metav1.Time{})); d != "" {
			return fmt.Errorf("unexpected tortoise: diff = %s", d)
		}
	}
	return nil
}

// createResources creates the resources defined in t.before.
func (t *testCase) createResources(ctx context.Context, k8sClient client.Client, config *rest.Config) error {
	err := k8sClient.Create(ctx, t.before.deployment.DeepCopy())
	if err != nil {
		return err
	}
	err = k8sClient.Status().Update(ctx, t.before.deployment.DeepCopy())
	if err != nil {
		return err
	}
	if t.before.hpa == nil {
		// create default HPA.
		HpaClient := hpa.New(k8sClient, 0.95, 90)
		t.before.hpa, t.before.tortoise, err = HpaClient.CreateHPAOnTortoise(ctx, t.before.tortoise, t.before.deployment)
		if err != nil {
			return err
		}
	}
	if t.before.vpa == nil {
		// create default VPAs.
		t.before.vpa = map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{}
		VpaClient, err := vpa.New(config)
		if err != nil {
			return err
		}
		vpacli, err := versioned.NewForConfig(config)
		if err != nil {
			return err
		}
		t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleUpdater], t.before.tortoise, err = VpaClient.CreateTortoiseUpdaterVPA(ctx, t.before.tortoise)
		if err != nil {
			return err
		}
		t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor], t.before.tortoise, err = VpaClient.CreateTortoiseMonitorVPA(ctx, t.before.tortoise)
		if err != nil {
			return err
		}
		r := make([]autoscalingv1.RecommendedContainerResources, len(t.before.deployment.Spec.Template.Spec.Containers))
		for i, c := range t.before.deployment.Spec.Template.Spec.Containers {
			r[i] = autoscalingv1.RecommendedContainerResources{
				ContainerName: c.Name,
				Target: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("3"),
					corev1.ResourceMemory: resource.MustParse("3Gi"),
				},
				LowerBound: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("3"),
					corev1.ResourceMemory: resource.MustParse("3Gi"),
				},
				UpperBound: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("5"),
					corev1.ResourceMemory: resource.MustParse("5Gi"),
				},
			}
		}
		t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor].Status.Recommendation = &autoscalingv1.RecommendedPodResources{
			ContainerRecommendations: r,
		}

		_, err = vpacli.AutoscalingV1().VerticalPodAutoscalers(t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor].Namespace).UpdateStatus(ctx, t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor], metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	err = k8sClient.Create(ctx, t.before.tortoise.DeepCopy())
	if err != nil {
		return err
	}
	tortoise := &v1alpha1.Tortoise{}
	err = k8sClient.Get(ctx, client.ObjectKey{Namespace: t.before.tortoise.Namespace, Name: t.before.tortoise.Name}, tortoise)
	if err != nil {
		panic(err)
	}
	tortoise.Status = t.before.tortoise.DeepCopy().Status
	err = k8sClient.Status().Update(ctx, tortoise)
	if err != nil {
		return err
	}
	return nil
}

func deploymentWithReplicaNum(replica int32) *v1.Deployment {
	return &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mercari-app",
			Namespace: "default",
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "mercari"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "mercari"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "awesome-mercari-app-image",
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			},
		},
		Status: v1.DeploymentStatus{
			Replicas: replica,
		},
	}
}

func resourceQuantityPtr(quantity resource.Quantity) *resource.Quantity {
	return &quantity
}
