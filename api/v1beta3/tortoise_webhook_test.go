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

package v1beta3

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

func mutateTest(before, after, deployment, hpa string) {
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

	if hpa != "" {
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
	}

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

func validateCreationTest(tortoise, hpa, deployment string, valid bool) {
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

	if hpa != "" {
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
	}

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

func validateUpdateTest(tortoise, existingTortoise, hpa, deployment string, valid bool) {
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

	y, err = os.ReadFile(existingTortoise)
	Expect(err).NotTo(HaveOccurred())
	tortoiseObj := &Tortoise{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(tortoiseObj)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, tortoiseObj)

	y, err = os.ReadFile(tortoise)
	Expect(err).NotTo(HaveOccurred())
	tortoiseObj = &Tortoise{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(tortoiseObj)
	Expect(err).NotTo(HaveOccurred())

	t := &Tortoise{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: tortoiseObj.GetName(), Namespace: tortoiseObj.GetNamespace()}, t)
	Expect(err).NotTo(HaveOccurred())
	t.Spec = tortoiseObj.Spec
	err = k8sClient.Update(ctx, t)

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

var _ = Describe("Tortoise Webhook", func() {
	Context("mutating", func() {
		It("nothing to do", func() {
			mutateTest(filepath.Join("testdata", "mutating", "nothing-to-do", "before.yaml"), filepath.Join("testdata", "mutating", "nothing-to-do", "after.yaml"), filepath.Join("testdata", "mutating", "nothing-to-do", "deployment.yaml"), "")
		})
		It("nothing to do (empty autoscaling policy)", func() {
			mutateTest(filepath.Join("testdata", "mutating", "no-autoscaling-policy", "before.yaml"), filepath.Join("testdata", "mutating", "no-autoscaling-policy", "after.yaml"), filepath.Join("testdata", "mutating", "no-autoscaling-policy", "deployment.yaml"), "")
		})
		It("should mutate a Tortoise with istio", func() {
			mutateTest(filepath.Join("testdata", "mutating", "with-istio", "before.yaml"), filepath.Join("testdata", "mutating", "with-istio", "after.yaml"), filepath.Join("testdata", "mutating", "with-istio", "deployment.yaml"), "")
		})
		It("should mutate a Tortoise which some autoscalingPolicy is specified, but not all", func() {
			mutateTest(filepath.Join("testdata", "mutating", "some-specified-others-not", "before.yaml"), filepath.Join("testdata", "mutating", "some-specified-others-not", "after.yaml"), filepath.Join("testdata", "mutating", "some-specified-others-not", "deployment.yaml"), "")
		})
	})
	Context("validating(creation)", func() {
		It("should create a valid Tortoise", func() {
			validateCreationTest(filepath.Join("testdata", "validating", "success", "tortoise.yaml"), filepath.Join("testdata", "validating", "success", "hpa.yaml"), filepath.Join("testdata", "validating", "success", "deployment.yaml"), true)
		})
		It("should create a valid Tortoise for the deployment with istio", func() {
			validateCreationTest(filepath.Join("testdata", "validating", "success-with-istio", "tortoise.yaml"), filepath.Join("testdata", "validating", "success-with-istio", "hpa.yaml"), filepath.Join("testdata", "validating", "success-with-istio", "deployment.yaml"), true)
		})
		It("invalid: Tortoise is targetting the resource other than Deployment", func() {
			validateCreationTest(filepath.Join("testdata", "validating", "not-targetting-deployment", "tortoise.yaml"), filepath.Join("testdata", "validating", "not-targetting-deployment", "hpa.yaml"), filepath.Join("testdata", "validating", "not-targetting-deployment", "deployment.yaml"), false)
		})
		It("invalid: Tortoise has resource policy for non-existing container", func() {
			validateCreationTest(filepath.Join("testdata", "validating", "useless-policy", "tortoise.yaml"), filepath.Join("testdata", "validating", "useless-policy", "hpa.yaml"), filepath.Join("testdata", "validating", "useless-policy", "deployment.yaml"), false)
		})
	})
	Context("validating(updating)", func() {
		It("should update a valid Tortoise", func() {
			validateUpdateTest(filepath.Join("testdata", "validating", "success", "tortoise.yaml"), filepath.Join("testdata", "validating", "success", "tortoise.yaml"), filepath.Join("testdata", "validating", "success", "hpa.yaml"), filepath.Join("testdata", "validating", "success", "deployment.yaml"), true)
		})
		It("should update a valid Tortoise for the deployment with istio", func() {
			validateUpdateTest(filepath.Join("testdata", "validating", "success-with-istio", "tortoise.yaml"), filepath.Join("testdata", "validating", "success-with-istio", "tortoise.yaml"), filepath.Join("testdata", "validating", "success-with-istio", "hpa.yaml"), filepath.Join("testdata", "validating", "success-with-istio", "deployment.yaml"), true)
		})
		It("successfully remove horizontal", func() {
			validateUpdateTest(filepath.Join("testdata", "validating", "success-remove-all-horizontal", "updating-tortoise.yaml"), filepath.Join("testdata", "validating", "success-remove-all-horizontal", "before-tortoise.yaml"), filepath.Join("testdata", "validating", "success-remove-all-horizontal", "hpa.yaml"), filepath.Join("testdata", "validating", "success-remove-all-horizontal", "deployment.yaml"), true)
		})
		It("no horizontal policy exists and the deletion policy is NoDelete", func() {
			validateUpdateTest(filepath.Join("testdata", "validating", "no-horizontal-with-no-deletion", "updating-tortoise.yaml"), filepath.Join("testdata", "validating", "no-horizontal-with-no-deletion", "before-tortoise.yaml"), filepath.Join("testdata", "validating", "no-horizontal-with-no-deletion", "hpa.yaml"), filepath.Join("testdata", "validating", "no-horizontal-with-no-deletion", "deployment.yaml"), false)
		})
		It("no horizontal policy exists and HPA is specified", func() {
			validateUpdateTest(filepath.Join("testdata", "validating", "no-horizontal-with-hpa", "updating-tortoise.yaml"), filepath.Join("testdata", "validating", "no-horizontal-with-hpa", "before-tortoise.yaml"), filepath.Join("testdata", "validating", "no-horizontal-with-no-deletion", "hpa.yaml"), filepath.Join("testdata", "validating", "no-horizontal-with-no-deletion", "deployment.yaml"), false)
		})
	})
})
