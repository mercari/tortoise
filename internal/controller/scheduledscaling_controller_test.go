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
		It("Should create successfully", func() {
			By("Creating a new ScheduledScaling")
			ctx := context.Background()
			resourceName := "test-scheduledscaling"

			resource := &autoscalingv1alpha1.ScheduledScaling{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: autoscalingv1alpha1.ScheduledScalingSpec{
					Schedule: autoscalingv1alpha1.Schedule{
						FinishAt: func() *string { v := "2024-12-31T23:59:59Z"; return &v }(),
					},
					TargetRefs: autoscalingv1alpha1.TargetRefs{
						TortoiseName: func() *string { v := "test-tortoise"; return &v }(),
					},
					Strategy: autoscalingv1alpha1.Strategy{
						Static: &autoscalingv1alpha1.Static{
							MinimumMinReplicas: func() *int { v := 1; return &v }(),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).Should(Succeed())

			// Let's make sure it was created in the cluster
			resourceLookupKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			createdResource := &autoscalingv1alpha1.ScheduledScaling{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, resourceLookupKey, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Let's wait until the resource is reconciled
			Eventually(func() bool {
				err := k8sClient.Get(ctx, resourceLookupKey, createdResource)
				// Add any specific conditions you want to check
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Perform cleanup
			By("Cleaning up the created resource")
			Expect(k8sClient.Delete(ctx, createdResource)).Should(Succeed())
		})
	})
})
