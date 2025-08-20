/*
Copyright 2024 The Tortoise Authors.

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

package controller

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"
	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ScheduledScaling Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a ScheduledScaling", func() {
		It("Should handle basic scheduled scaling creation", func() {
			ctx := context.Background()

			// Create a test ScheduledScaling resource
			scheduledScaling := &autoscalingv1alpha1.ScheduledScaling{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scheduled-scaling",
					Namespace: "default",
				},
				Spec: autoscalingv1alpha1.ScheduledScalingSpec{
					Schedule: autoscalingv1alpha1.Schedule{
						StartAt:  time.Now().Add(time.Hour).Format(time.RFC3339),
						FinishAt: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
					},
					TargetRefs: autoscalingv1alpha1.TargetRefs{
						TortoiseName: "test-tortoise",
					},
					Strategy: autoscalingv1alpha1.Strategy{
						Static: autoscalingv1alpha1.StaticStrategy{
							MinimumMinReplicas: 3,
							MinAllocatedResources: autoscalingv1alpha1.ResourceRequirements{
								CPU:    "500m",
								Memory: "512Mi",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, scheduledScaling)).Should(Succeed())

			// Verify the resource was created and wait for status update
			resourceLookupKey := types.NamespacedName{
				Name:      scheduledScaling.Name,
				Namespace: scheduledScaling.Namespace,
			}
			createdResource := &autoscalingv1alpha1.ScheduledScaling{}

			// Wait for the controller to reconcile and set the status to Pending
			Eventually(func() autoscalingv1alpha1.ScheduledScalingPhase {
				err := k8sClient.Get(ctx, resourceLookupKey, createdResource)
				if err != nil {
					return ""
				}
				return createdResource.Status.Phase
			}, timeout, interval).Should(Equal(autoscalingv1alpha1.ScheduledScalingPhasePending))

			// Clean up
			Expect(k8sClient.Delete(ctx, scheduledScaling)).Should(Succeed())
		})

		It("Should reject invalid schedule times", func() {
			ctx := context.Background()

			// Create a ScheduledScaling with invalid times (finish before start)
			scheduledScaling := &autoscalingv1alpha1.ScheduledScaling{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid-schedule",
					Namespace: "default",
				},
				Spec: autoscalingv1alpha1.ScheduledScalingSpec{
					Schedule: autoscalingv1alpha1.Schedule{
						StartAt:  time.Now().Add(2 * time.Hour).Format(time.RFC3339),
						FinishAt: time.Now().Add(time.Hour).Format(time.RFC3339), // Invalid: finish before start
					},
					TargetRefs: autoscalingv1alpha1.TargetRefs{
						TortoiseName: "test-tortoise",
					},
					Strategy: autoscalingv1alpha1.Strategy{
						Static: autoscalingv1alpha1.StaticStrategy{
							MinimumMinReplicas: 3,
							MinAllocatedResources: autoscalingv1alpha1.ResourceRequirements{
								CPU:    "500m",
								Memory: "512Mi",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, scheduledScaling)).Should(Succeed())

			// Wait for the controller to process and update status
			resourceLookupKey := types.NamespacedName{
				Name:      scheduledScaling.Name,
				Namespace: scheduledScaling.Namespace,
			}
			createdResource := &autoscalingv1alpha1.ScheduledScaling{}

			Eventually(func() autoscalingv1alpha1.ScheduledScalingPhase {
				err := k8sClient.Get(ctx, resourceLookupKey, createdResource)
				if err != nil {
					return ""
				}
				return createdResource.Status.Phase
			}, timeout, interval).Should(Equal(autoscalingv1alpha1.ScheduledScalingPhaseFailed))

			// Verify the error reason
			Expect(createdResource.Status.Reason).Should(Equal("InvalidSchedule"))

			// Clean up
			Expect(k8sClient.Delete(ctx, scheduledScaling)).Should(Succeed())
		})
	})

	Context("Apply and restore scheduled scaling", func() {
		It("should apply desired min resources and then restore original spec", func() {
			ctx := context.Background()

			// Create a baseline Tortoise that ScheduledScaling will target
			t := &autoscalingv1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tortoise-apply", Namespace: "default"},
				Spec: autoscalingv1beta3.TortoiseSpec{
					ResourcePolicy: []autoscalingv1beta3.ContainerResourcePolicy{
						{
							ContainerName: "app",
							MinAllocatedResources: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, t)).Should(Succeed())

			// Prepare a ScheduledScaling that is currently active and ends soon
			start := time.Now().Add(-1 * time.Second).Format(time.RFC3339)
			finish := time.Now().Add(3 * time.Second).Format(time.RFC3339)
			ss := &autoscalingv1alpha1.ScheduledScaling{
				ObjectMeta: metav1.ObjectMeta{Name: "test-apply-restore", Namespace: "default"},
				Spec: autoscalingv1alpha1.ScheduledScalingSpec{
					Schedule:   autoscalingv1alpha1.Schedule{StartAt: start, FinishAt: finish},
					TargetRefs: autoscalingv1alpha1.TargetRefs{TortoiseName: t.Name},
					Strategy: autoscalingv1alpha1.Strategy{Static: autoscalingv1alpha1.StaticStrategy{
						MinimumMinReplicas:    5,
						MinAllocatedResources: autoscalingv1alpha1.ResourceRequirements{CPU: "500m", Memory: "512Mi"},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, ss)).Should(Succeed())

			// During active window, Tortoise should be updated
			Eventually(func(g Gomega) {
				cur := &autoscalingv1beta3.Tortoise{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, cur)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(cur.Spec.ResourcePolicy).ToNot(BeEmpty())
				pol := cur.Spec.ResourcePolicy[0]
				cpu := pol.MinAllocatedResources[v1.ResourceCPU]
				mem := pol.MinAllocatedResources[v1.ResourceMemory]
				g.Expect((&cpu).Cmp(resource.MustParse("500m")) >= 0).To(BeTrue())
				g.Expect((&mem).Cmp(resource.MustParse("512Mi")) >= 0).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			// After the window finishes, original spec should be restored
			Eventually(func(g Gomega) {
				cur := &autoscalingv1beta3.Tortoise{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, cur)
				g.Expect(err).ShouldNot(HaveOccurred())
				pol := cur.Spec.ResourcePolicy[0]
				g.Expect(pol.MinAllocatedResources[v1.ResourceCPU].Equal(resource.MustParse("100m"))).To(BeTrue())
				g.Expect(pol.MinAllocatedResources[v1.ResourceMemory].Equal(resource.MustParse("128Mi"))).To(BeTrue())
			}, time.Second*12, interval).Should(Succeed())
		})
	})
})
