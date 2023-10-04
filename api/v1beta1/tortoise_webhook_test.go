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

package v1beta1

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"

	v1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func mutateTest(before, after, deployment string) {
	ctx := context.Background()

	y, err := os.ReadFile(deployment)
	Expect(err).NotTo(HaveOccurred())
	deploy := &v1.Deployment{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(deploy)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, deploy)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		err = k8sClient.Delete(ctx, deploy)
		Expect(err).NotTo(HaveOccurred())
	}()

	y, err = os.ReadFile(before)
	Expect(err).NotTo(HaveOccurred())
	beforeTortoise := &Tortoise{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(beforeTortoise)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, beforeTortoise)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		err = k8sClient.Delete(ctx, beforeTortoise)
		Expect(err).NotTo(HaveOccurred())
	}()

	ret := &Tortoise{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: beforeTortoise.GetName(), Namespace: beforeTortoise.GetNamespace()}, ret)
	Expect(err).NotTo(HaveOccurred())

	y, err = os.ReadFile(after)
	Expect(err).NotTo(HaveOccurred())
	afterTortoise := &Tortoise{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(afterTortoise)
	Expect(err).NotTo(HaveOccurred())

	Expect(ret.Spec).Should(Equal(afterTortoise.Spec))
}

func validateTest(tortoise, hpa, deployment string, valid bool) {
	ctx := context.Background()

	y, err := os.ReadFile(deployment)
	Expect(err).NotTo(HaveOccurred())
	deploy := &v1.Deployment{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(deploy)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, deploy)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		err = k8sClient.Delete(ctx, deploy)
		Expect(err).NotTo(HaveOccurred())
	}()

	y, err = os.ReadFile(hpa)
	Expect(err).NotTo(HaveOccurred())
	hpaObj := &autoscalingv2.HorizontalPodAutoscaler{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(hpaObj)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, hpaObj)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		err = k8sClient.Delete(ctx, hpaObj)
		Expect(err).NotTo(HaveOccurred())
	}()

	y, err = os.ReadFile(tortoise)
	Expect(err).NotTo(HaveOccurred())
	tortoiseObj := &Tortoise{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(tortoiseObj)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, tortoiseObj)

	if valid {
		Expect(err).NotTo(HaveOccurred(), "Tortoise: %v", tortoiseObj)

		// cleanup
		err = k8sClient.Delete(ctx, tortoiseObj)
		Expect(err).NotTo(HaveOccurred())
	} else {
		Expect(err).To(HaveOccurred(), "Tortoise: %v", tortoiseObj)
		statusErr := &apierrors.StatusError{}
		Expect(errors.As(err, &statusErr)).To(BeTrue())
		expected := tortoiseObj.Annotations["message"]
		Expect(statusErr.ErrStatus.Message).To(ContainSubstring(expected))
	}
}

var _ = Describe("MarkdownTortoise Webhook", func() {
	Context("mutating", func() {
		It("should mutate a Tortoise", func() {
			mutateTest(filepath.Join("testdata", "mutating", "before.yaml"), filepath.Join("testdata", "mutating", "after.yaml"), filepath.Join("testdata", "mutating", "deployment.yaml"))
		})
	})
	Context("validating", func() {
		It("should create a valid Tortoise", func() {
			validateTest(filepath.Join("testdata", "validating", "success", "tortoise.yaml"), filepath.Join("testdata", "validating", "success", "hpa.yaml"), filepath.Join("testdata", "validating", "success", "deployment.yaml"), true)
		})
		It("invalid: Tortoise has Horizontal for the container, but HPA doens't have ContainerResource metric for that container", func() {
			validateTest(filepath.Join("testdata", "validating", "no-metric-in-hpa", "tortoise.yaml"), filepath.Join("testdata", "validating", "no-metric-in-hpa", "hpa.yaml"), filepath.Join("testdata", "validating", "no-metric-in-hpa", "deployment.yaml"), false)
		})
		It("invalid: HPA has ContainerResource metric for the container, but autoscalingPolicy in tortoise isn't Horizontal", func() {
			validateTest(filepath.Join("testdata", "validating", "no-horizontal-in-tortoise", "tortoise.yaml"), filepath.Join("testdata", "validating", "no-horizontal-in-tortoise", "hpa.yaml"), filepath.Join("testdata", "validating", "no-horizontal-in-tortoise", "deployment.yaml"), false)
		})
		It("invalid: Tortoise has resource policy for non-existing container", func() {
			validateTest(filepath.Join("testdata", "validating", "useless-policy", "tortoise.yaml"), filepath.Join("testdata", "validating", "useless-policy", "hpa.yaml"), filepath.Join("testdata", "validating", "useless-policy", "deployment.yaml"), false)
		})
	})
})
