package workload

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/mercari/tortoise/pkg/annotation"
)

func podTemplate(cpu string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "mercari"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "awesome-mercari-app-image",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse(cpu),
						},
					},
				},
			},
		},
	}
}

func deployment(name string, replicas *int32, cpu string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "mercari"}},
			Template: podTemplate(cpu),
		},
	}
}

func rollout(t *testing.T, spec map[string]interface{}) *unstructured.Unstructured {
	t.Helper()
	r := &unstructured.Unstructured{Object: map[string]interface{}{"spec": spec}}
	r.SetGroupVersionKind(RolloutGVK)
	r.SetName("mercari-app")
	r.SetNamespace("default")
	return r
}

func rolloutWithTemplate(t *testing.T, replicas *int64, cpu string) *unstructured.Unstructured {
	t.Helper()
	tmpl := podTemplate(cpu)
	tm, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&tmpl)
	if err != nil {
		t.Fatal(err)
	}
	spec := map[string]interface{}{
		"template": tm,
		// A field Tortoise doesn't know about, which must be preserved.
		"strategy": map[string]interface{}{
			"canary": map[string]interface{}{
				"steps": []interface{}{map[string]interface{}{"setWeight": int64(20)}},
			},
		},
	}
	if replicas != nil {
		spec["replicas"] = *replicas
	}
	return rollout(t, spec)
}

func rolloutWithWorkloadRef(t *testing.T, kind, name string) *unstructured.Unstructured {
	t.Helper()
	return rollout(t, map[string]interface{}{
		"replicas": int64(5),
		"workloadRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"name":       name,
		},
	})
}

func newClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(objs...).Build()
}

func getRollout(t *testing.T, c client.Client) *unstructured.Unstructured {
	t.Helper()
	r := &unstructured.Unstructured{}
	r.SetGroupVersionKind(RolloutGVK)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "mercari-app"}, r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestGet(t *testing.T) {
	tests := []struct {
		name         string
		objs         []client.Object
		kind         string
		wantErr      bool
		wantReplicas *int32
		wantTemplate corev1.PodTemplateSpec
	}{
		{
			name:         "Deployment",
			objs:         []client.Object{deployment("mercari-app", ptr.To[int32](3), "1")},
			kind:         KindDeployment,
			wantReplicas: ptr.To[int32](3),
			wantTemplate: podTemplate("1"),
		},
		{
			name:         "Rollout with the inline template",
			objs:         []client.Object{rolloutWithTemplate(t, ptr.To[int64](4), "2")},
			kind:         KindRollout,
			wantReplicas: ptr.To[int32](4),
			wantTemplate: podTemplate("2"),
		},
		{
			name:         "Rollout without replicas is regarded as having one replica",
			objs:         []client.Object{rolloutWithTemplate(t, nil, "2")},
			kind:         KindRollout,
			wantReplicas: ptr.To[int32](1),
			wantTemplate: podTemplate("2"),
		},
		{
			name: "Rollout referring to Deployment via workloadRef",
			objs: []client.Object{
				rolloutWithWorkloadRef(t, KindDeployment, "mercari-app-template"),
				deployment("mercari-app-template", ptr.To[int32](0), "3"),
			},
			kind: KindRollout,
			// The number of replicas is the one of Rollout, not the referenced Deployment.
			wantReplicas: ptr.To[int32](5),
			wantTemplate: podTemplate("3"),
		},
		{
			name:    "Rollout referring to unsupported kind via workloadRef",
			objs:    []client.Object{rolloutWithWorkloadRef(t, "PodTemplate", "mercari-app-template")},
			kind:    KindRollout,
			wantErr: true,
		},
		{
			name: "Rollout referring to non-existing Deployment via workloadRef",
			objs: []client.Object{rolloutWithWorkloadRef(t, KindDeployment, "mercari-app-template")},
			kind: KindRollout,
			// The referenced Deployment doesn't exist.
			wantErr: true,
		},
		{
			name:    "not found",
			kind:    KindDeployment,
			wantErr: true,
		},
		{
			name:    "unsupported kind",
			objs:    []client.Object{deployment("mercari-app", ptr.To[int32](3), "1")},
			kind:    "StatefulSet",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := Get(context.Background(), newClient(tt.objs...), "default", tt.kind, "mercari-app")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if w.Kind() != tt.kind {
				t.Errorf("Kind() = %v, want %v", w.Kind(), tt.kind)
			}
			if w.Object().GetName() != "mercari-app" {
				t.Errorf("Object().GetName() = %v, want mercari-app", w.Object().GetName())
			}
			replicas, err := w.Replicas()
			if err != nil {
				t.Fatalf("Replicas() error = %v", err)
			}
			if d := cmp.Diff(tt.wantReplicas, replicas); d != "" {
				t.Errorf("Replicas() diff = %s", d)
			}
			if d := cmp.Diff(tt.wantTemplate, *w.PodTemplate()); d != "" {
				t.Errorf("PodTemplate() diff = %s", d)
			}
		})
	}
}

func TestRestart(t *testing.T) {
	now := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Deployment gets the annotation on the Pod template", func(t *testing.T) {
		c := newClient(deployment("mercari-app", ptr.To[int32](3), "1"))
		w, err := Get(context.Background(), c, "default", KindDeployment, "mercari-app")
		if err != nil {
			t.Fatal(err)
		}
		if err := Restart(context.Background(), c, w, now); err != nil {
			t.Fatal(err)
		}

		got := &appsv1.Deployment{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "mercari-app"}, got); err != nil {
			t.Fatal(err)
		}
		if v := got.Spec.Template.Annotations[annotation.UpdatedAtAnnotation]; v != "2023-01-01T00:00:00Z" {
			t.Errorf("annotation %s = %q, want %q", annotation.UpdatedAtAnnotation, v, "2023-01-01T00:00:00Z")
		}
	})

	t.Run("Rollout gets .spec.restartAt, and the Pod template isn't changed", func(t *testing.T) {
		original := rolloutWithTemplate(t, ptr.To[int64](3), "1")
		c := newClient(original.DeepCopy())
		w, err := Get(context.Background(), c, "default", KindRollout, "mercari-app")
		if err != nil {
			t.Fatal(err)
		}
		if err := Restart(context.Background(), c, w, now); err != nil {
			t.Fatal(err)
		}

		got := getRollout(t, c)
		restartAt, _, _ := unstructured.NestedString(got.Object, "spec", "restartAt")
		if restartAt != "2023-01-01T00:00:00Z" {
			t.Errorf(".spec.restartAt = %q, want %q", restartAt, "2023-01-01T00:00:00Z")
		}
		unstructured.RemoveNestedField(got.Object, "spec", "restartAt")
		if d := cmp.Diff(original.Object["spec"], got.Object["spec"]); d != "" {
			t.Errorf("unexpected change in .spec: diff = %s", d)
		}
	})

	t.Run("Rollout referring to Deployment via workloadRef gets .spec.restartAt, and the Deployment isn't changed", func(t *testing.T) {
		dp := deployment("mercari-app-template", ptr.To[int32](0), "1")
		c := newClient(rolloutWithWorkloadRef(t, KindDeployment, "mercari-app-template"), dp.DeepCopy())
		w, err := Get(context.Background(), c, "default", KindRollout, "mercari-app")
		if err != nil {
			t.Fatal(err)
		}
		if err := Restart(context.Background(), c, w, now); err != nil {
			t.Fatal(err)
		}

		got := getRollout(t, c)
		restartAt, _, _ := unstructured.NestedString(got.Object, "spec", "restartAt")
		if restartAt != "2023-01-01T00:00:00Z" {
			t.Errorf(".spec.restartAt = %q, want %q", restartAt, "2023-01-01T00:00:00Z")
		}
		gotDP := &appsv1.Deployment{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "mercari-app-template"}, gotDP); err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(dp.Spec, gotDP.Spec); d != "" {
			t.Errorf("unexpected change in the referenced Deployment: diff = %s", d)
		}
	})
}

func TestUpdatePodTemplate(t *testing.T) {
	t.Run("Deployment", func(t *testing.T) {
		c := newClient(deployment("mercari-app", ptr.To[int32](3), "1"))
		w, err := Get(context.Background(), c, "default", KindDeployment, "mercari-app")
		if err != nil {
			t.Fatal(err)
		}
		w.PodTemplate().Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("2")
		if err := UpdatePodTemplate(context.Background(), c, w); err != nil {
			t.Fatal(err)
		}

		got := &appsv1.Deployment{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "mercari-app"}, got); err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(podTemplate("2"), got.Spec.Template); d != "" {
			t.Errorf("unexpected Pod template: diff = %s", d)
		}
	})

	t.Run("Rollout with the inline template; unknown fields are preserved", func(t *testing.T) {
		original := rolloutWithTemplate(t, ptr.To[int64](3), "1")
		c := newClient(original.DeepCopy())
		w, err := Get(context.Background(), c, "default", KindRollout, "mercari-app")
		if err != nil {
			t.Fatal(err)
		}
		w.PodTemplate().Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("2")
		if err := UpdatePodTemplate(context.Background(), c, w); err != nil {
			t.Fatal(err)
		}

		got := getRollout(t, c)
		updated, err := NewFromRollout(got, nil)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(podTemplate("2"), *updated.PodTemplate()); d != "" {
			t.Errorf("unexpected Pod template: diff = %s", d)
		}
		if d := cmp.Diff(original.Object["spec"].(map[string]interface{})["strategy"], got.Object["spec"].(map[string]interface{})["strategy"]); d != "" {
			t.Errorf(".spec.strategy is changed: diff = %s", d)
		}
		replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
		if replicas != 3 {
			t.Errorf(".spec.replicas = %d, want 3", replicas)
		}
	})

	t.Run("Rollout referring to Deployment via workloadRef updates the Deployment", func(t *testing.T) {
		original := rolloutWithWorkloadRef(t, KindDeployment, "mercari-app-template")
		c := newClient(original.DeepCopy(), deployment("mercari-app-template", ptr.To[int32](0), "1"))
		w, err := Get(context.Background(), c, "default", KindRollout, "mercari-app")
		if err != nil {
			t.Fatal(err)
		}
		w.PodTemplate().Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("2")
		if err := UpdatePodTemplate(context.Background(), c, w); err != nil {
			t.Fatal(err)
		}

		gotDP := &appsv1.Deployment{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "mercari-app-template"}, gotDP); err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(podTemplate("2"), gotDP.Spec.Template); d != "" {
			t.Errorf("unexpected Pod template in the referenced Deployment: diff = %s", d)
		}
		got := getRollout(t, c)
		if d := cmp.Diff(original.Object["spec"], got.Object["spec"]); d != "" {
			t.Errorf("Rollout is changed: diff = %s", d)
		}
	})
}
