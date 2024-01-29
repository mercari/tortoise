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

package autoscalingv2

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	v2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/mercari/tortoise/api/v1beta3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func mutateTest(before, after, tortoise string) {
	ctx := context.Background()

	y, err := os.ReadFile(tortoise)
	Expect(err).NotTo(HaveOccurred())
	tor := &v1beta3.Tortoise{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(tor)
	status := tor.Status
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, tor.DeepCopy())
	Expect(err).NotTo(HaveOccurred())

	err = k8sClient.Get(ctx, types.NamespacedName{Name: tor.GetName(), Namespace: tor.GetNamespace()}, tor)
	Expect(err).NotTo(HaveOccurred())
	tor.Status = status
	err = k8sClient.Status().Update(ctx, tor)
	Expect(err).NotTo(HaveOccurred())

	y, err = os.ReadFile(before)
	Expect(err).NotTo(HaveOccurred())
	beforehpa := &v2.HorizontalPodAutoscaler{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(beforehpa)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, beforehpa)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		// cleanup
		err = k8sClient.Delete(ctx, tor)
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(time.Second)
		err = k8sClient.Delete(ctx, beforehpa)
		Expect(err).NotTo(HaveOccurred())
	}()

	ret := &v2.HorizontalPodAutoscaler{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: beforehpa.GetName(), Namespace: beforehpa.GetNamespace()}, ret)
	Expect(err).NotTo(HaveOccurred())

	y, err = os.ReadFile(after)
	Expect(err).NotTo(HaveOccurred())
	afterhpa := &v2.HorizontalPodAutoscaler{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(afterhpa)
	Expect(err).NotTo(HaveOccurred())

	Expect(ret.Spec).Should(Equal(afterhpa.Spec))
}

func validateDeletionTest(hpa, tortoise string, valid bool) {
	ctx := context.Background()

	var tor *v1beta3.Tortoise
	if tortoise != "" {
		y, err := os.ReadFile(tortoise)
		Expect(err).NotTo(HaveOccurred())
		tor = &v1beta3.Tortoise{}
		err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(tor)
		status := tor.Status
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Create(ctx, tor.DeepCopy())
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Get(ctx, types.NamespacedName{Name: tor.GetName(), Namespace: tor.GetNamespace()}, tor)
		Expect(err).NotTo(HaveOccurred())
		tor.Status = status
		err = k8sClient.Status().Update(ctx, tor)
		Expect(err).NotTo(HaveOccurred())
	}

	y, err := os.ReadFile(hpa)
	Expect(err).NotTo(HaveOccurred())
	beforehpa := &v2.HorizontalPodAutoscaler{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(beforehpa)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, beforehpa)
	Expect(err).NotTo(HaveOccurred())

	defer func() {
		// cleanup
		if tortoise != "" {
			err = k8sClient.Delete(ctx, tor)
			Expect(err).NotTo(HaveOccurred())
		}
		if !valid { // if valid, HPA is already deleted
			err = k8sClient.Delete(ctx, beforehpa)
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	ret := &v2.HorizontalPodAutoscaler{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: beforehpa.GetName(), Namespace: beforehpa.GetNamespace()}, ret)
	Expect(err).NotTo(HaveOccurred())

	err = k8sClient.Delete(ctx, beforehpa)
	if valid {
		Expect(err).NotTo(HaveOccurred(), "Tortoise: %v", beforehpa)
	} else {
		Expect(err).To(HaveOccurred(), "Tortoise: %v", beforehpa)
		statusErr := &apierrors.StatusError{}
		Expect(errors.As(err, &statusErr)).To(BeTrue())
		expected := beforehpa.Annotations["message"]
		Expect(statusErr.ErrStatus.Message).To(ContainSubstring(expected))
	}
}

var _ = Describe("v2.HPA Webhook", func() {
	Context("mutating", func() {
		It("HPA is mutated based on the recommendation", func() {
			mutateTest(filepath.Join("testdata", "mutating", "mutate-by-recommendations", "before.yaml"), filepath.Join("testdata", "mutating", "mutate-by-recommendations", "after.yaml"), filepath.Join("testdata", "mutating", "mutate-by-recommendations", "tortoise.yaml"))
		})
		It("HPA is partly mutated based on the recommendation", func() {
			mutateTest(filepath.Join("testdata", "mutating", "mutate-by-one-recommendation", "before.yaml"), filepath.Join("testdata", "mutating", "mutate-by-one-recommendation", "after.yaml"), filepath.Join("testdata", "mutating", "mutate-by-one-recommendation", "tortoise.yaml"))
		})
		It("HPA is not mutated (dryrun)", func() {
			mutateTest(filepath.Join("testdata", "mutating", "no-mutate-by-recommendations-when-dryrun", "before.yaml"), filepath.Join("testdata", "mutating", "no-mutate-by-recommendations-when-dryrun", "after.yaml"), filepath.Join("testdata", "mutating", "no-mutate-by-recommendations-when-dryrun", "tortoise.yaml"))
		})
		It("HPA is not mutated because of invalid annotation", func() {
			mutateTest(filepath.Join("testdata", "mutating", "has-annotation-but-invalid1", "before.yaml"), filepath.Join("testdata", "mutating", "has-annotation-but-invalid1", "after.yaml"), filepath.Join("testdata", "mutating", "has-annotation-but-invalid1", "tortoise.yaml"))
			mutateTest(filepath.Join("testdata", "mutating", "has-annotation-but-invalid2", "before.yaml"), filepath.Join("testdata", "mutating", "has-annotation-but-invalid2", "after.yaml"), filepath.Join("testdata", "mutating", "has-annotation-but-invalid2", "tortoise.yaml"))
		})
	})
	Context("validating", func() {
		It("valid: HPA can be deleted when Tortoise (Off) exists", func() {
			validateDeletionTest(filepath.Join("testdata", "validating", "hpa-with-off", "hpa.yaml"), filepath.Join("testdata", "validating", "hpa-with-off", "tortoise.yaml"), true)
		})
		It("valid: HPA can be deleted when Tortoise (Auto) is deleted", func() {
			validateDeletionTest(filepath.Join("testdata", "validating", "hpa-with-auto-deleted", "hpa.yaml"), "", true)
		})
		It("invalid: HPA cannot be deleted when Tortoise (Auto) exists", func() {
			validateDeletionTest(filepath.Join("testdata", "validating", "hpa-with-auto-existing", "hpa.yaml"), filepath.Join("testdata", "validating", "hpa-with-auto-existing", "tortoise.yaml"), false)
		})
		It("valid: HPA can be deleted when Tortoise (Auto) is being deleted", func() {
			// create tortoise
			y, err := os.ReadFile(filepath.Join("testdata", "validating", "hpa-with-auto-being-deleted", "tortoise.yaml"))
			Expect(err).NotTo(HaveOccurred())
			tor := &v1beta3.Tortoise{}
			err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(tor)
			// add finalizer
			controllerutil.AddFinalizer(tor, tortoiseFinalizer)
			status := tor.Status
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Create(ctx, tor.DeepCopy())
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: tor.GetName(), Namespace: tor.GetNamespace()}, tor)
			Expect(err).NotTo(HaveOccurred())
			tor.Status = status
			err = k8sClient.Status().Update(ctx, tor)
			Expect(err).NotTo(HaveOccurred())

			// delete tortoise, but it shouldn't be deleted because of the finalizer
			err = k8sClient.Delete(ctx, tor)
			Expect(err).NotTo(HaveOccurred())

			// create HPA
			y, err = os.ReadFile(filepath.Join("testdata", "validating", "hpa-with-auto-being-deleted", "hpa.yaml"))
			Expect(err).NotTo(HaveOccurred())
			beforehpa := &v2.HorizontalPodAutoscaler{}
			err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(beforehpa)
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Create(ctx, beforehpa)
			Expect(err).NotTo(HaveOccurred())

			// try to delete HPA
			err = k8sClient.Delete(ctx, beforehpa)
			Expect(err).NotTo(HaveOccurred(), "Tortoise: %v", beforehpa)

			// remove finalizer and remove tortoise (cleanup)
			controllerutil.RemoveFinalizer(tor, tortoiseFinalizer)
			err = k8sClient.Delete(ctx, tor)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

const tortoiseFinalizer = "tortoise.autoscaling.mercari.com/finalizer"
