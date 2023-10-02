package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mercari/tortoise/api/v1alpha1"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/recommender"
	"github.com/mercari/tortoise/pkg/tortoise"
	"github.com/mercari/tortoise/pkg/utils"
	"github.com/mercari/tortoise/pkg/vpa"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test TortoiseController", func() {
	ctx := context.Background()
	var stopFunc func()
	cleanUp := func() {
		err := deleteObj(ctx, &v1alpha1.Tortoise{}, "mercari")
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

	BeforeEach(func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ShouldNot(HaveOccurred())

		tortoiseService, err := tortoise.New(mgr.GetClient(), 1, "Asia/Tokyo", 1000*time.Minute, "weekly")
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
		suiteConfig, _ := GinkgoConfiguration()
		if CurrentSpecReport().Failed() && suiteConfig.FailFast {
			suiteFailed = true
		} else {
			cleanUp()
			for {
				// make sure all resources are deleted
				t := &v1alpha1.Tortoise{}
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
			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName: "mercari-app",
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
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// create the desired HPA from the created definition.
			wantHPA := tc.before.hpa.DeepCopy()
			wantHPA.Spec.MinReplicas = pointer.Int32(5)
			wantHPA.Spec.MaxReplicas = 20
			for i, m := range wantHPA.Spec.Metrics {
				if m.ContainerResource != nil && m.ContainerResource.Name == corev1.ResourceCPU && m.ContainerResource.Container == "app" {
					wantHPA.Spec.Metrics[i].ContainerResource.Target.AverageUtilization = pointer.Int32(75)
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
						DeploymentName: "mercari-app",
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
						HorizontalPodAutoscaler: "tortoise-hpa-mercari",
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
									WeekDay:   pointer.String(now.Weekday().String()),
									TimeZone:  now.Location().String(),
									Value:     20,
									UpdatedAt: metav1.NewTime(now),
								},
							},
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   pointer.String(now.Weekday().String()),
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
				vpa: wantVPA,
			}
			tc.want = want
			Eventually(func(g Gomega) {
				gotTortoise := &v1alpha1.Tortoise{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotHPA := &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
				g.Expect(err).ShouldNot(HaveOccurred())

				err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA, vpa: map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
					v1alpha1.VerticalPodAutoscalerRoleUpdater: gotUpdaterVPA,
					v1alpha1.VerticalPodAutoscalerRoleMonitor: gotMonitorVPA,
				}})
				g.Expect(err).ShouldNot(HaveOccurred())
			}).Should(Succeed())
		})
		It("TortoisePhaseWorking (dryrun)", func() {
			// When dryrun, Tortoise is updated, but HPA and VPA are not updated.

			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetUpdateMode(v1alpha1.UpdateModeOff).
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName: "mercari-app",
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
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// create the desired HPA from the created definition.
			// (no difference from "before")
			wantHPA := tc.before.hpa.DeepCopy()
			// create the desired VPA from the created definition.
			// (no difference from "before")
			wantVPA := map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{}
			wantUpdater := tc.before.vpa[v1alpha1.VerticalPodAutoscalerRoleUpdater].DeepCopy()
			wantVPA[v1alpha1.VerticalPodAutoscalerRoleUpdater] = wantUpdater
			wantVPA[v1alpha1.VerticalPodAutoscalerRoleMonitor] = tc.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor].DeepCopy()

			want := resources{
				tortoise: utils.NewTortoiseBuilder().
					SetName("mercari").
					SetNamespace("default").
					SetUpdateMode(v1alpha1.UpdateModeOff).
					SetTargetRefs(v1alpha1.TargetRefs{
						DeploymentName: "mercari-app",
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
						HorizontalPodAutoscaler: "tortoise-hpa-mercari",
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
									WeekDay:   pointer.String(now.Weekday().String()),
									TimeZone:  now.Location().String(),
									Value:     20,
									UpdatedAt: metav1.NewTime(now),
								},
							},
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   pointer.String(now.Weekday().String()),
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
				vpa: wantVPA,
			}
			tc.want = want
			Eventually(func(g Gomega) {
				gotTortoise := &v1alpha1.Tortoise{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotHPA := &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
				g.Expect(err).ShouldNot(HaveOccurred())

				err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA, vpa: map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
					v1alpha1.VerticalPodAutoscalerRoleUpdater: gotUpdaterVPA,
					v1alpha1.VerticalPodAutoscalerRoleMonitor: gotMonitorVPA,
				}})
				g.Expect(err).ShouldNot(HaveOccurred())
			}).Should(Succeed())
		})
		It("TortoisePhaseEmergency", func() {
			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName: "mercari-app",
						}).
						SetUpdateMode(v1alpha1.UpdateModeEmergency).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						SetTortoisePhase(v1alpha1.TortoisePhaseWorking). // will be updated.
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
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// create the desired HPA from the created definition.
			wantHPA := tc.before.hpa.DeepCopy()
			wantHPA.Spec.MinReplicas = pointer.Int32(20)
			wantHPA.Spec.MaxReplicas = 20
			for i, m := range wantHPA.Spec.Metrics {
				if m.ContainerResource != nil && m.ContainerResource.Name == corev1.ResourceCPU && m.ContainerResource.Container == "app" {
					wantHPA.Spec.Metrics[i].ContainerResource.Target.AverageUtilization = pointer.Int32(75)
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
						DeploymentName: "mercari-app",
					}).
					SetUpdateMode(v1alpha1.UpdateModeEmergency).
					AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
						ContainerName: "app",
						AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
							corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
						},
					}).
					SetTortoisePhase(v1alpha1.TortoisePhaseEmergency).
					SetTargetsStatus(v1alpha1.TargetsStatus{
						HorizontalPodAutoscaler: "tortoise-hpa-mercari",
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
									WeekDay:   pointer.String(now.Weekday().String()),
									TimeZone:  now.Location().String(),
									Value:     20,
									UpdatedAt: metav1.NewTime(now),
								},
							},
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   pointer.String(now.Weekday().String()),
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
				vpa: wantVPA,
			}
			tc.want = want
			Eventually(func(g Gomega) {
				gotTortoise := &v1alpha1.Tortoise{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotHPA := &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
				g.Expect(err).ShouldNot(HaveOccurred())

				err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA, vpa: map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
					v1alpha1.VerticalPodAutoscalerRoleUpdater: gotUpdaterVPA,
					v1alpha1.VerticalPodAutoscalerRoleMonitor: gotMonitorVPA,
				}})
				g.Expect(err).ShouldNot(HaveOccurred())
			}).Should(Succeed())
		})
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
							DeploymentName: "mercari-app",
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "istio-proxy",
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
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[corev1.ResourceName]int32{
											corev1.ResourceCPU: 50, // will be updated.
										},
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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
						AddCondition(v1alpha1.ContainerRecommendationFromVPA{
							ContainerName: "istio-proxy",
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
					deployment: multiContainerDeploymentWithReplicaNum(10),
				},
			}

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// create the desired HPA from the created definition.
			wantHPA := tc.before.hpa.DeepCopy()
			wantHPA.Spec.MinReplicas = pointer.Int32(5)
			wantHPA.Spec.MaxReplicas = 20
			for i, m := range wantHPA.Spec.Metrics {
				if m.ContainerResource != nil && m.ContainerResource.Name == corev1.ResourceCPU && m.ContainerResource.Container == "app" {
					wantHPA.Spec.Metrics[i].ContainerResource.Target.AverageUtilization = pointer.Int32(50) // won't get changed.
				}
				if m.ContainerResource != nil && m.ContainerResource.Name == corev1.ResourceCPU && m.ContainerResource.Container == "istio-proxy" {
					wantHPA.Spec.Metrics[i].ContainerResource.Target.AverageUtilization = pointer.Int32(75)
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
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					LowerBound: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					UpperBound: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					UncappedTarget: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
				},
				{
					ContainerName: "istio-proxy",
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
						DeploymentName: "mercari-app",
					}).
					AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
						ContainerName: "app",
						AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
							corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
						},
					}).
					AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
						ContainerName: "istio-proxy",
						AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
							corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
						},
					}).
					SetTortoisePhase(v1alpha1.TortoisePhaseWorking).
					SetTargetsStatus(v1alpha1.TargetsStatus{
						HorizontalPodAutoscaler: "tortoise-hpa-mercari",
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
										corev1.ResourceCPU:    resource.MustParse("6"),
										corev1.ResourceMemory: resource.MustParse("3Gi"),
									},
								},
								{
									ContainerName: "istio-proxy",
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
										corev1.ResourceCPU:    50,
										corev1.ResourceMemory: 90,
									},
								},
								{
									ContainerName: "istio-proxy",
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
									WeekDay:   pointer.String(now.Weekday().String()),
									TimeZone:  now.Location().String(),
									Value:     20,
									UpdatedAt: metav1.NewTime(now),
								},
							},
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   pointer.String(now.Weekday().String()),
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
					AddCondition(v1alpha1.ContainerRecommendationFromVPA{
						ContainerName: "istio-proxy",
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
				vpa: wantVPA,
			}
			tc.want = want
			Eventually(func(g Gomega) {
				gotTortoise := &v1alpha1.Tortoise{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotHPA := &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
				g.Expect(err).ShouldNot(HaveOccurred())

				err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA, vpa: map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
					v1alpha1.VerticalPodAutoscalerRoleUpdater: gotUpdaterVPA,
					v1alpha1.VerticalPodAutoscalerRoleMonitor: gotMonitorVPA,
				}})
				g.Expect(err).ShouldNot(HaveOccurred())
			}).Should(Succeed())
		})
		It("TortoisePhaseEmergency", func() {
			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName: "mercari-app",
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "istio-proxy",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						SetUpdateMode(v1alpha1.UpdateModeEmergency).
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
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[corev1.ResourceName]int32{
											corev1.ResourceCPU: 50, // will be updated.
										},
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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
						AddCondition(v1alpha1.ContainerRecommendationFromVPA{
							ContainerName: "istio-proxy",
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
					deployment: multiContainerDeploymentWithReplicaNum(10),
				},
			}

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// create the desired HPA from the created definition.
			wantHPA := tc.before.hpa.DeepCopy()
			wantHPA.Spec.MinReplicas = pointer.Int32(20)
			wantHPA.Spec.MaxReplicas = 20
			for i, m := range wantHPA.Spec.Metrics {
				if m.ContainerResource != nil && m.ContainerResource.Name == corev1.ResourceCPU && m.ContainerResource.Container == "app" {
					wantHPA.Spec.Metrics[i].ContainerResource.Target.AverageUtilization = pointer.Int32(50) // won't get changed.
				}
				if m.ContainerResource != nil && m.ContainerResource.Name == corev1.ResourceCPU && m.ContainerResource.Container == "istio-proxy" {
					wantHPA.Spec.Metrics[i].ContainerResource.Target.AverageUtilization = pointer.Int32(75)
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
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					LowerBound: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					UpperBound: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					UncappedTarget: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
				},
				{
					ContainerName: "istio-proxy",
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
						DeploymentName: "mercari-app",
					}).
					AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
						ContainerName: "app",
						AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
							corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
						},
					}).
					AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
						ContainerName: "istio-proxy",
						AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
							corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
							corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
						},
					}).
					SetUpdateMode(v1alpha1.UpdateModeEmergency).
					SetTortoisePhase(v1alpha1.TortoisePhaseEmergency).
					SetTargetsStatus(v1alpha1.TargetsStatus{
						HorizontalPodAutoscaler: "tortoise-hpa-mercari",
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
										corev1.ResourceCPU:    resource.MustParse("6"),
										corev1.ResourceMemory: resource.MustParse("3Gi"),
									},
								},
								{
									ContainerName: "istio-proxy",
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
										corev1.ResourceCPU:    50,
										corev1.ResourceMemory: 90,
									},
								},
								{
									ContainerName: "istio-proxy",
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
									WeekDay:   pointer.String(now.Weekday().String()),
									TimeZone:  now.Location().String(),
									Value:     20,
									UpdatedAt: metav1.NewTime(now),
								},
							},
							MinReplicas: []v1alpha1.ReplicasRecommendation{
								{
									From:      0,
									To:        24,
									WeekDay:   pointer.String(now.Weekday().String()),
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
					AddCondition(v1alpha1.ContainerRecommendationFromVPA{
						ContainerName: "istio-proxy",
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
				vpa: wantVPA,
			}
			tc.want = want
			Eventually(func(g Gomega) {
				gotTortoise := &v1alpha1.Tortoise{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, gotTortoise)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotHPA := &v2.HorizontalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, gotHPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotUpdaterVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, gotUpdaterVPA)
				g.Expect(err).ShouldNot(HaveOccurred())
				gotMonitorVPA := &autoscalingv1.VerticalPodAutoscaler{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, gotMonitorVPA)
				g.Expect(err).ShouldNot(HaveOccurred())

				err = tc.compare(resources{tortoise: gotTortoise, hpa: gotHPA, vpa: map[v1alpha1.VerticalPodAutoscalerRole]*autoscalingv1.VerticalPodAutoscaler{
					v1alpha1.VerticalPodAutoscalerRoleUpdater: gotUpdaterVPA,
					v1alpha1.VerticalPodAutoscalerRoleMonitor: gotMonitorVPA,
				}})
				g.Expect(err).ShouldNot(HaveOccurred())
			}).Should(Succeed())
		})
	})
	Context("DeletionPolicy is handled correctly", func() {
		It("[DeletionPolicy = DeleteAll] delete HPA and VPAs when Tortoise is deleted", func() {
			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetDeletionPolicy(v1alpha1.DeletionPolicyDeleteAll).
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName: "mercari-app",
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "istio-proxy",
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
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[corev1.ResourceName]int32{
											corev1.ResourceCPU: 50, // will be updated.
										},
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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
						AddCondition(v1alpha1.ContainerRecommendationFromVPA{
							ContainerName: "istio-proxy",
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
					deployment: multiContainerDeploymentWithReplicaNum(10),
				},
			}

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())
			time.Sleep(1 * time.Second)

			// delete Tortoise
			err = k8sClient.Delete(ctx, tc.before.tortoise)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func(g Gomega) {
				// make sure all resources are deleted
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, &v1alpha1.Tortoise{})
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
			now := time.Now()
			tc := testCase{
				before: resources{
					tortoise: utils.NewTortoiseBuilder().
						SetName("mercari").
						SetNamespace("default").
						SetDeletionPolicy(v1alpha1.DeletionPolicyNoDelete).
						SetTargetRefs(v1alpha1.TargetRefs{
							DeploymentName: "mercari-app",
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "app",
							AutoscalingPolicy: map[corev1.ResourceName]v1alpha1.AutoscalingType{
								corev1.ResourceCPU:    v1alpha1.AutoscalingTypeHorizontal,
								corev1.ResourceMemory: v1alpha1.AutoscalingTypeVertical,
							},
						}).
						AddResourcePolicy(v1alpha1.ContainerResourcePolicy{
							ContainerName: "istio-proxy",
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
									{
										ContainerName: "istio-proxy",
										TargetUtilization: map[corev1.ResourceName]int32{
											corev1.ResourceCPU: 50, // will be updated.
										},
									},
								},
								MaxReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
										TimeZone:  now.Location().String(),
										Value:     15, // will be updated
										UpdatedAt: metav1.NewTime(now),
									},
								},
								MinReplicas: []v1alpha1.ReplicasRecommendation{
									{
										From:      0,
										To:        24,
										WeekDay:   pointer.String(now.Weekday().String()),
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
						AddCondition(v1alpha1.ContainerRecommendationFromVPA{
							ContainerName: "istio-proxy",
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
					deployment: multiContainerDeploymentWithReplicaNum(10),
				},
			}

			err := tc.initializeResources(ctx, k8sClient, cfg)
			Expect(err).ShouldNot(HaveOccurred())

			// delete Tortoise
			err = k8sClient.Delete(ctx, tc.before.tortoise)
			Expect(err).ShouldNot(HaveOccurred())

			// wait for the reconciliation
			time.Sleep(1 * time.Second)

			Eventually(func(g Gomega) {
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-hpa-mercari"}, &v2.HorizontalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-updater-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "tortoise-monitor-mercari"}, &autoscalingv1.VerticalPodAutoscaler{})
				Expect(err).ShouldNot(HaveOccurred())
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "mercari"}, &v1alpha1.Tortoise{})
				g.Expect(apierrors.IsNotFound(err)).To(Equal(true))
			}).Should(Succeed())
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

// initializeResources creates the resources defined in t.before.
func (t *testCase) initializeResources(ctx context.Context, k8sClient client.Client, config *rest.Config) error {
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
		t.before.hpa, t.before.tortoise, err = HpaClient.CreateHPA(ctx, t.before.tortoise, t.before.deployment)
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
		t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor].Status.Conditions = []autoscalingv1.VerticalPodAutoscalerCondition{
			{
				Type:   autoscalingv1.RecommendationProvided,
				Status: corev1.ConditionTrue,
			},
		}

		_, err = vpacli.AutoscalingV1().VerticalPodAutoscalers(t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor].Namespace).UpdateStatus(ctx, t.before.vpa[v1alpha1.VerticalPodAutoscalerRoleMonitor], metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	err = k8sClient.Create(ctx, t.before.tortoise.DeepCopy())
	if err != nil {
		return fmt.Errorf("failed to create tortoise: %w", err)
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		tortoise := &v1alpha1.Tortoise{}
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: t.before.tortoise.Namespace, Name: t.before.tortoise.Name}, tortoise)
		if err != nil {
			return fmt.Errorf("failed to get tortoise: %w", err)
		}
		tortoise.Status = t.before.tortoise.DeepCopy().Status
		err = k8sClient.Status().Update(ctx, tortoise)
		if err != nil {
			return fmt.Errorf("failed to update tortoise status: %w", err)
		}
		return nil
	})
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

func multiContainerDeploymentWithReplicaNum(replica int32) *v1.Deployment {
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
									corev1.ResourceCPU:    resource.MustParse("10"),
									corev1.ResourceMemory: resource.MustParse("10Gi"),
								},
							},
						},
						{
							Name:  "istio-proxy",
							Image: "awesome-istio-proxy-image",
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
