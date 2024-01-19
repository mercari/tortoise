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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	//+kubebuilder:scaffold:imports
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/config"
	"github.com/mercari/tortoise/pkg/hpa"
	"github.com/mercari/tortoise/pkg/tortoise"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Webhook Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")

	y, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "webhook", "manifests.yaml"))
	Expect(err).NotTo(HaveOccurred())
	mutatingWebhookConfig := &admissionv1.MutatingWebhookConfiguration{}
	validatingWebhookConfig := &admissionv1.ValidatingWebhookConfiguration{}
	d := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096)
	err = d.Decode(mutatingWebhookConfig)
	Expect(err).NotTo(HaveOccurred())
	err = d.Decode(validatingWebhookConfig)
	Expect(err).NotTo(HaveOccurred())

	for i, w := range mutatingWebhookConfig.Webhooks {
		for _, rule := range w.Rules {
			for _, resource := range rule.Resources {
				// We want not to include the mutating webhook for tortoise.
				if resource == "tortoises" {
					mutatingWebhookConfig.Webhooks = append(mutatingWebhookConfig.Webhooks[:i], mutatingWebhookConfig.Webhooks[i+1:]...)
				}
			}
		}
	}

	for i, w := range validatingWebhookConfig.Webhooks {
		for _, rule := range w.Rules {
			for _, resource := range rule.Resources {
				// We want not to include the mutating webhook for tortoise.
				if resource == "tortoises" {
					validatingWebhookConfig.Webhooks = append(validatingWebhookConfig.Webhooks[:i], validatingWebhookConfig.Webhooks[i+1:]...)
				}
			}
		}
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths:              []string{filepath.Join("..", "..", "..", "config", "webhook", "service.yaml")},
			MutatingWebhooks:   []*admissionv1.MutatingWebhookConfiguration{mutatingWebhookConfig},
			ValidatingWebhooks: []*admissionv1.ValidatingWebhookConfiguration{validatingWebhookConfig},
		},
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	err = v1beta3.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = autoscalingv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = autoscalingv2.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = admissionv1beta1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())
	config, err := config.ParseConfig("")
	Expect(err).NotTo(HaveOccurred())
	eventRecorder := mgr.GetEventRecorderFor("tortoise-controller")
	tortoiseService, err := tortoise.New(mgr.GetClient(), eventRecorder, config.RangeOfMinMaxReplicasRecommendationHours, config.TimeZone, config.TortoiseUpdateInterval, config.GatheringDataPeriodType)
	Expect(err).NotTo(HaveOccurred())
	hpaService, err := hpa.New(mgr.GetClient(), eventRecorder, config.ReplicaReductionFactor, config.UpperTargetResourceUtilization, 100, time.Hour, 1000, 10000, "")
	Expect(err).NotTo(HaveOccurred())

	hpaWebhook := New(tortoiseService, hpaService)

	err = ctrl.NewWebhookManagedBy(mgr).
		WithDefaulter(hpaWebhook).
		WithValidator(hpaWebhook).
		For(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete()
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:webhook

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
