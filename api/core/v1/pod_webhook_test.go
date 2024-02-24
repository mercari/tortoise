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

package v1

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

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
	beforepod := &v1.Pod{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(beforepod)
	Expect(err).NotTo(HaveOccurred())
	err = k8sClient.Create(ctx, beforepod)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		// cleanup
		err = k8sClient.Delete(ctx, tor)
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(time.Second)
		err = k8sClient.Delete(ctx, beforepod)
		Expect(err).NotTo(HaveOccurred())
	}()

	ret := &v1.Pod{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: beforepod.GetName(), Namespace: beforepod.GetNamespace()}, ret)
	Expect(err).NotTo(HaveOccurred())

	y, err = os.ReadFile(after)
	Expect(err).NotTo(HaveOccurred())
	afterpod := &v1.Pod{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096).Decode(afterpod)
	Expect(err).NotTo(HaveOccurred())

	diff := cmp.Diff(ret.Spec.Containers, afterpod.Spec.Containers)
	Expect(diff).To(Equal(""), "diff: %s", diff)
}

var _ = Describe("v1.Pod Webhook", func() {
	Context("mutating", func() {
		It("Pod with Auto Tortoise is mutated", func() {
			mutateTest(filepath.Join("testdata", "mutating", "auto-tortoise", "before.yaml"), filepath.Join("testdata", "mutating", "auto-tortoise", "after.yaml"), filepath.Join("testdata", "mutating", "auto-tortoise", "tortoise.yaml"))
		})
		It("Pod with Off Tortoise is not mutated", func() {
			mutateTest(filepath.Join("testdata", "mutating", "off-tortoise", "before.yaml"), filepath.Join("testdata", "mutating", "off-tortoise", "after.yaml"), filepath.Join("testdata", "mutating", "off-tortoise", "tortoise.yaml"))
		})
	})
})
