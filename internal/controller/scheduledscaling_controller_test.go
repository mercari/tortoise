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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoscalingv1alpha1 "github.com/mercari/tortoise/api/v1alpha1"

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

			// Verify the resource was created
			resourceLookupKey := types.NamespacedName{
				Name:      scheduledScaling.Name,
				Namespace: scheduledScaling.Namespace,
			}
			createdResource := &autoscalingv1alpha1.ScheduledScaling{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, resourceLookupKey, createdResource)
				// Add any specific conditions you want to check
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Verify the status is set to Pending initially
			Expect(createdResource.Status.Phase).Should(Equal(autoscalingv1alpha1.ScheduledScalingPhasePending))

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
})
