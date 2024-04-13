package tortoisectl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

func buildTortoiseCtl(t *testing.T) {
	t.Helper()

	_, _, err := execCommand(t, "go", "build", "-o", "./testdata/bin/tortoisectl", "../main.go")
	if err != nil {
		t.Fatalf("Failed to build tortoisectl: %v", err)
	}
}

func prepareCluster(t *testing.T) (*envtest.Environment, *rest.Config) {
	t.Helper()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// cfg is defined in this file globally.
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("Failed to start test environment: %v", err)
	}

	return testEnv, cfg
}

func destryCluster(t *testing.T, testEnv *envtest.Environment) {
	t.Helper()

	err := testEnv.Stop()
	if err != nil {
		t.Fatalf("Failed to destroy test environment: %v", err)
	}
}

func Test_TortoiseCtlStop(t *testing.T) {
	update := false
	if os.Getenv("UPDATE_TESTCASES") == "true" {
		update = true
	}

	// Build the latest binary.
	buildTortoiseCtl(t)
	testEnv, cfg := prepareCluster(t)
	defer destryCluster(t, testEnv)

	// create the clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create clientset: %v", err)
	}

	scheme := runtime.NewScheme()
	err = v1beta3.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}
	kubeconfig := strings.Split(testEnv.ControlPlane.KubeCtl().Opts[0], "=")[1]

	tortoiseclient, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tests := []struct {
		name string
		// tortoisectl stop
		options         []string
		dir             string // dir is also the namespace
		expectedFailure bool
	}{
		{
			name: "stop tortoise successfully",
			options: []string{
				"--namespace", "success", "mercaritortoise",
			},
			dir: "success",
		},
		{
			name: "stop tortoise successfully with --no-lowering-resources",
			options: []string{
				"--namespace", "success-no-lowering-resources", "--no-lowering-resources", "mercaritortoise",
			},
			dir: "success-no-lowering-resources",
		},
		{
			name: "stop tortoise successfully with --no-lowering-resources (istio annotation)",
			options: []string{
				"--namespace", "success-no-lowering-resources", "--no-lowering-resources", "mercaritortoise",
			},
			dir: "success-no-lowering-resources-w-istio",
		},
		{
			name: "stop all tortoises in a namespace successfully with --all",
			options: []string{
				"--namespace", "success-all-in-namespace", "--all",
			},
			dir: "success-all-in-namespace",
		},
		{
			name: "stop all tortoises in all namespace successfully with --all",
			options: []string{
				"--all",
			},
			dir: "success-all-in-all-namespace",
		},
		{
			name: "cannot use --all and specify a tortoise name at the same time",
			options: []string{
				"--namespace", "success", "--all", "mercaritortoise",
			},
			dir:             "success",
			expectedFailure: true,
		},
		{
			name: "have to specify a tortoise name if no --all",
			options: []string{
				"--namespace", "success",
			},
			dir:             "success",
			expectedFailure: true,
		},
		{
			name: "have to specify a namespace if no --all",
			options: []string{
				"hoge",
			},
			dir:             "success",
			expectedFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespaces := map[string]struct{}{}
			t.Cleanup(func() {
				// remove all namespaces
				for namespace := range namespaces {
					deployments, err := clientset.AppsV1().Deployments(namespace).List(context.Background(), metav1.ListOptions{})
					if err != nil {
						t.Fatalf("Failed to list deployments: %v", err)
					}
					for _, deploy := range deployments.Items {
						err = clientset.AppsV1().Deployments(namespace).Delete(context.Background(), deploy.Name, metav1.DeleteOptions{})
						if err != nil {
							t.Fatalf("Failed to delete deployment: %v", err)
						}
					}

					tortoises := &v1beta3.TortoiseList{}
					err = tortoiseclient.List(context.Background(), tortoises, &client.ListOptions{Namespace: namespace})
					if err != nil {
						t.Fatalf("Failed to list tortoises: %v", err)
					}
					for _, tortoise := range tortoises.Items {
						err = tortoiseclient.Delete(context.Background(), &tortoise)
						if err != nil {
							t.Fatalf("Failed to delete tortoise: %v", err)
						}
					}

					err = clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
					if err != nil {
						t.Fatalf("Failed to delete namespace: %v", err)
					}

					ns, err := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
					if err != nil {
						t.Fatalf("Failed to get namespace: %v", err)
					}

					ns.TypeMeta = metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					}
					ns.Spec.Finalizers = nil
					patch, err := json.Marshal(ns)
					if err != nil {
						t.Fatalf("Failed to marshal namespace: %v", err)
					}
					result := clientset.RESTClient().Put().AbsPath(fmt.Sprintf("/api/v1/namespaces/%s/finalize", namespace)).Body(patch).Do(context.Background())
					if result.Error() != nil {
						t.Fatalf("Failed to update namespace: %v", result.Error())
					}

				}

				for namespace := range namespaces {
					wait.PollUntilContextCancel(context.Background(), 500*time.Millisecond, true, func(ctx context.Context) (bool, error) {
						_, err := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
						return err != nil, nil
					})
				}
			})

			deploymentNameToFileName := map[client.ObjectKey]string{}
			originalDeployments := map[client.ObjectKey]appv1.Deployment{}
			deploymentDir := fmt.Sprintf("./testdata/%s/before/deployments", tt.dir)
			err = filepath.Walk(deploymentDir, func(deploymentYaml string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				y, err := os.ReadFile(deploymentYaml)
				if err != nil {
					t.Fatalf("Failed to read deployment yaml: %v", err)
				}
				deploy := &appv1.Deployment{}
				err = yaml.Unmarshal(y, deploy)
				if err != nil {
					t.Fatalf("Failed to unmarshal deployment yaml: %v", err)
				}

				namespace := deploy.Namespace
				if _, ok := namespaces[namespace]; !ok {
					// Create a namespace
					_, err = clientset.CoreV1().Namespaces().Create(context.Background(), &v1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: namespace,
						},
					}, metav1.CreateOptions{})
					if err != nil {
						t.Fatalf("Failed to create namespace: %v", err)
					}

					namespaces[namespace] = struct{}{}
				}

				// Create a deployment
				_, err = clientset.AppsV1().Deployments(namespace).Create(context.Background(), deploy, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create deployment: %v", err)
				}

				originalDeployments[client.ObjectKey{
					Namespace: deploy.Namespace,
					Name:      deploy.Name,
				}] = *deploy

				deploymentNameToFileName[client.ObjectKey{
					Namespace: deploy.Namespace,
					Name:      deploy.Name,
				}] = filepath.Base(deploymentYaml)

				return nil
			})

			tortoiseNameToFileName := map[client.ObjectKey]string{}
			tortoiseDir := fmt.Sprintf("./testdata/%s/before/tortoises", tt.dir)
			err = filepath.Walk(tortoiseDir, func(tortoiseYaml string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}

				y, err := os.ReadFile(tortoiseYaml)
				if err != nil {
					t.Fatalf("Failed to read tortoise yaml: %v", err)
				}

				tortoise := &v1beta3.Tortoise{}
				err = yaml.Unmarshal(y, tortoise)
				if err != nil {
					t.Fatalf("Failed to unmarshal tortoise yaml: %v", err)
				}

				status := tortoise.DeepCopy().Status
				err = tortoiseclient.Create(context.Background(), tortoise)
				if err != nil {
					t.Fatalf("Failed to create tortoise: %v", err)
				}

				v := &v1beta3.Tortoise{}
				err = tortoiseclient.Get(context.Background(), client.ObjectKey{Namespace: tortoise.Namespace, Name: tortoise.Name}, v)
				if err != nil {
					t.Fatalf("Failed to get tortoise: %v", err)
				}
				v.Status = status
				err = tortoiseclient.Status().Update(context.Background(), v)
				if err != nil {
					t.Fatalf("Failed to update tortoise status: %v", err)
				}

				tortoiseNameToFileName[client.ObjectKey{
					Namespace: tortoise.Namespace,
					Name:      tortoise.Name,
				}] = filepath.Base(tortoiseYaml)

				return nil
			})

			stdout, stderr, err := execCommand(t, append([]string{"./testdata/bin/tortoisectl", "stop", "--kubeconfig", kubeconfig}, tt.options...)...)
			if err != nil {
				if !tt.expectedFailure {
					t.Fatalf("Failed to run tortoisectl stop: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
				}

				// If the test is expected to fail, we don't need to check the result of the resources.
				return
			}

			t.Log(stdout)

			gotDeployments := map[client.ObjectKey]appv1.Deployment{}
			for k := range deploymentNameToFileName {
				deploy, err := clientset.AppsV1().Deployments(k.Namespace).Get(context.Background(), k.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("Failed to get deployment: %v", err)
				}
				gotDeployments[k] = *deploy
			}

			gotTortoises := map[client.ObjectKey]v1beta3.Tortoise{}
			for k := range tortoiseNameToFileName {
				tortoise := &v1beta3.Tortoise{}
				err = tortoiseclient.Get(context.Background(), k, tortoise)
				if err != nil {
					t.Fatalf("Failed to get tortoise: %v", err)
				}

				gotTortoises[k] = *tortoise
			}

			if update {
				deploymentDir := fmt.Sprintf("./testdata/%s/after/deployments", tt.dir)
				for key, filename := range deploymentNameToFileName {
					if gotDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] !=
						originalDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] {
						gotDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] = "updated"
					}

					err = writeToFile(filepath.Join(deploymentDir, filename), gotDeployments[key])
					if err != nil {
						t.Fatalf("Failed to write deployment yaml: %v", err)
					}
				}

				tortoiseDir := fmt.Sprintf("./testdata/%s/after/tortoises", tt.dir)
				for key, filename := range tortoiseNameToFileName {
					err = writeToFile(filepath.Join(tortoiseDir, filename), gotTortoises[key])
					if err != nil {
						t.Fatalf("Failed to write tortoise yaml: %v", err)
					}
				}

				return
			}

			for key, filename := range tortoiseNameToFileName {
				tortoisePath := filepath.Join(fmt.Sprintf("./testdata/%s/after/tortoises", tt.dir), filename)
				y, err := os.ReadFile(tortoisePath)
				if err != nil {
					t.Fatalf("Failed to read tortoise yaml: %v", err)
				}
				wantTortoise := &v1beta3.Tortoise{}
				err = yaml.Unmarshal(y, wantTortoise)
				if err != nil {
					t.Fatalf("Failed to decode tortoise yaml: %v", err)
				}

				diff := cmp.Diff(*wantTortoise, gotTortoises[key], cmpopts.IgnoreFields(v1beta3.Tortoise{}, "ObjectMeta"))
				if diff != "" {
					t.Fatalf("Tortoise %v mismatch (-want +got):\n%s", key, diff)
				}
			}

			for key, filename := range deploymentNameToFileName {
				deploymentPath := filepath.Join(fmt.Sprintf("./testdata/%s/after/deployments", tt.dir), filename)
				y, err := os.ReadFile(deploymentPath)
				if err != nil {
					t.Fatalf("Failed to read deployment yaml: %v", err)
				}
				wantDeployment := &appv1.Deployment{}
				err = yaml.Unmarshal(y, wantDeployment)
				if err != nil {
					t.Fatalf("Failed to decode deployment yaml: %v", err)
				}

				switch wantDeployment.Spec.Template.Annotations[annotation.UpdatedAtAnnotation] {
				case "updated":
					// Check if the deployment is restarted (i.e., the annotation is changed).
					if gotDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] ==
						originalDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] {
						t.Fatalf("the target deployment %s is not restarted even though it should be", gotDeployments[key].Name)
					}

					wantDeployment.Spec.Template.Annotations[annotation.UpdatedAtAnnotation] = gotDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] // Update the annotation for comparison of Diff.
				default:
					// Check if the deployment is NOT restarted (i.e., the annotation is NOT changed).
					if gotDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] !=
						originalDeployments[key].Spec.Template.Annotations[annotation.UpdatedAtAnnotation] {
						t.Fatalf("the target deployment %s is restarted even though it should not be", gotDeployments[key].Name)
					}
				}

				diff := cmp.Diff(*wantDeployment, gotDeployments[key], cmpopts.IgnoreFields(appv1.Deployment{}, "ObjectMeta"))
				if diff != "" {
					t.Fatalf("Deployment %v mismatch (-want +got):\n%s", key, diff)
				}

				diff = cmp.Diff(wantDeployment.ObjectMeta.Annotations, gotDeployments[key].ObjectMeta.Annotations)
				if diff != "" {
					t.Fatalf("Deployment %v annotations mismatch (-want +got):\n%s", key, diff)
				}
			}
		})
	}
}

func writeToFile(path string, r any) error {
	y, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	y, err = removeUnnecessaryFields(y)
	if err != nil {
		return err
	}
	err = os.WriteFile(path, y, 0644)
	if err != nil {
		return err
	}

	return nil
}

func removeUnnecessaryFields(rawdata []byte) ([]byte, error) {
	data := make(map[string]interface{})
	err := yaml.Unmarshal(rawdata, &data)
	if err != nil {
		return nil, err
	}
	meta, ok := data["metadata"]
	if !ok {
		return nil, errors.New("no metadata")
	}
	typed, ok := meta.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("metadata is unexpected type: %T", meta)
	}

	delete(typed, "creationTimestamp")
	delete(typed, "managedFields")
	delete(typed, "resourceVersion")
	delete(typed, "uid")
	delete(typed, "generation")

	return yaml.Marshal(data)
}

func execCommand(t *testing.T, s ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(s[0], s[1:]...)
	t.Log(cmd.String())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	t.Logf("Running command: %s\n", cmd.String())
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
