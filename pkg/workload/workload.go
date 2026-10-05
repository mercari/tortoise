// Package workload provides an abstraction over the scale target workloads that Tortoise supports,
// i.e., Deployment and Argo Rollouts' Rollout.
//
// Argo Rollouts' Rollout is handled via unstructured.Unstructured
// so that Tortoise doesn't need to depend on the Argo Rollouts Go module,
// and so that updating a Rollout doesn't drop fields Tortoise doesn't know about.
package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mercari/tortoise/pkg/annotation"
)

const (
	KindDeployment = "Deployment"
	KindRollout    = "Rollout"

	DeploymentAPIVersion = "apps/v1"
	RolloutAPIVersion    = "argoproj.io/v1alpha1"
)

// RolloutGVK is the GroupVersionKind of Argo Rollouts' Rollout.
var RolloutGVK = schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: KindRollout}

// IsSupportedKind returns true if the kind is supported as the scale target of Tortoise.
func IsSupportedKind(kind string) bool {
	return kind == KindDeployment || kind == KindRollout
}

// DefaultAPIVersion returns the API version that Tortoise uses for the given kind.
// It returns an empty string if the kind isn't supported.
func DefaultAPIVersion(kind string) string {
	switch kind {
	case KindDeployment:
		return DeploymentAPIVersion
	case KindRollout:
		return RolloutAPIVersion
	}
	return ""
}

// Workload is the scale target of Tortoise.
type Workload struct {
	kind string

	// deployment is the target Deployment when kind is Deployment,
	// or the Deployment referenced by .spec.workloadRef when kind is Rollout and the Rollout uses workloadRef.
	deployment *appsv1.Deployment
	// rollout is the target Rollout when kind is Rollout.
	rollout *unstructured.Unstructured
	// template is the Pod template of the workload.
	// When the workload is a Rollout with the inline template, it's decoded from the Rollout and
	// the change on it is written back to the Rollout in UpdatePodTemplate.
	// Otherwise, it points to the template in deployment.
	template *corev1.PodTemplateSpec
}

// NewFromDeployment creates a Workload from the Deployment.
func NewFromDeployment(d *appsv1.Deployment) *Workload {
	return &Workload{kind: KindDeployment, deployment: d, template: &d.Spec.Template}
}

// NewFromRollout creates a Workload from the Rollout.
// referencedDeployment should be given when the Rollout refers to the Deployment via .spec.workloadRef.
func NewFromRollout(r *unstructured.Unstructured, referencedDeployment *appsv1.Deployment) (*Workload, error) {
	w := &Workload{kind: KindRollout, rollout: r}
	if referencedDeployment != nil {
		w.deployment = referencedDeployment
		w.template = &referencedDeployment.Spec.Template
		return w, nil
	}

	w.template = &corev1.PodTemplateSpec{}
	t, found, err := unstructured.NestedMap(r.Object, "spec", "template")
	if err != nil {
		return nil, fmt.Errorf("get .spec.template from rollout: %w", err)
	}
	if !found {
		return w, nil
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(t, w.template); err != nil {
		return nil, fmt.Errorf("convert .spec.template of rollout to PodTemplateSpec: %w", err)
	}
	return w, nil
}

// Kind returns the kind of the workload.
func (w *Workload) Kind() string {
	return w.kind
}

// Object returns the underlying object of the scale target.
func (w *Workload) Object() client.Object {
	if w.kind == KindRollout {
		return w.rollout
	}
	return w.deployment
}

// Replicas returns the desired number of replicas of the workload.
// It returns nil if the number of replicas isn't known.
func (w *Workload) Replicas() (*int32, error) {
	if w.kind != KindRollout {
		return w.deployment.Spec.Replicas, nil
	}

	r, found, err := unstructured.NestedInt64(w.rollout.Object, "spec", "replicas")
	if err != nil {
		return nil, fmt.Errorf("get .spec.replicas from rollout: %w", err)
	}
	if !found {
		// Argo Rollouts regards the Rollout without .spec.replicas as having one replica.
		return ptr.To[int32](1), nil
	}
	return ptr.To(int32(r)), nil
}

// PodTemplate returns the Pod template of the workload.
// When the workload is a Rollout referring to the Deployment via .spec.workloadRef, it's the Pod template of the Deployment.
// The caller can modify the returned template and write it back via UpdatePodTemplate.
func (w *Workload) PodTemplate() *corev1.PodTemplateSpec {
	return w.template
}

// Get gets the workload of the given kind and name.
func Get(ctx context.Context, c client.Client, namespace, kind, name string) (*Workload, error) {
	switch kind {
	case KindDeployment:
		d := &appsv1.Deployment{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, d); err != nil {
			return nil, fmt.Errorf("failed to get deployment: %w", err)
		}
		return NewFromDeployment(d), nil
	case KindRollout:
		r := &unstructured.Unstructured{}
		r.SetGroupVersionKind(RolloutGVK)
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, r); err != nil {
			return nil, fmt.Errorf("failed to get rollout: %w", err)
		}

		ref, found, err := unstructured.NestedStringMap(r.Object, "spec", "workloadRef")
		if err != nil {
			return nil, fmt.Errorf("get .spec.workloadRef from rollout: %w", err)
		}
		if !found {
			return NewFromRollout(r, nil)
		}
		if ref["kind"] != KindDeployment {
			return nil, fmt.Errorf("rollout %s/%s refers to %q via .spec.workloadRef, but only Deployment is supported", namespace, name, ref["kind"])
		}
		d := &appsv1.Deployment{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref["name"]}, d); err != nil {
			return nil, fmt.Errorf("failed to get deployment referred by rollout's .spec.workloadRef: %w", err)
		}
		return NewFromRollout(r, d)
	}

	return nil, fmt.Errorf("unsupported scale target kind: %s", kind)
}

// Restart restarts all the Pods of the workload.
//
// For a Deployment, it updates the annotation on the Pod template, which results in the rolling update.
// For a Rollout, it sets .spec.restartAt instead of changing the Pod template,
// because changing the Pod template creates a new revision and starts the whole canary/blue-green progression.
func Restart(ctx context.Context, c client.Client, w *Workload, now time.Time) error {
	if w.kind == KindRollout {
		// Use the merge patch, as `kubectl argo rollouts restart` does,
		// so that it doesn't conflict with the Argo Rollouts controller which updates the Rollout frequently.
		patch, err := json.Marshal(map[string]interface{}{
			"spec": map[string]interface{}{
				"restartAt": now.UTC().Format(time.RFC3339),
			},
		})
		if err != nil {
			return fmt.Errorf("marshal the patch for rollout: %w", err)
		}
		if err := c.Patch(ctx, w.rollout, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("failed to patch rollout: %w", err)
		}
		return nil
	}

	if w.deployment.Spec.Template.Annotations == nil {
		w.deployment.Spec.Template.Annotations = make(map[string]string)
	}
	w.deployment.Spec.Template.Annotations[annotation.UpdatedAtAnnotation] = now.Format(time.RFC3339)

	if err := c.Update(ctx, w.deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}
	return nil
}

// UpdatePodTemplate writes the Pod template, which is modified via PodTemplate(), back to the cluster.
// When the workload is a Rollout referring to the Deployment via .spec.workloadRef, the referenced Deployment is updated.
func UpdatePodTemplate(ctx context.Context, c client.Client, w *Workload) error {
	if w.kind == KindRollout && w.deployment == nil {
		t, err := runtime.DefaultUnstructuredConverter.ToUnstructured(w.template)
		if err != nil {
			return fmt.Errorf("convert PodTemplateSpec to unstructured: %w", err)
		}
		if v, found, _ := unstructured.NestedFieldNoCopy(t, "metadata", "creationTimestamp"); found && v == nil {
			// The conversion adds "creationTimestamp: null", which isn't in the original Rollout.
			unstructured.RemoveNestedField(t, "metadata", "creationTimestamp")
		}
		if err := unstructured.SetNestedMap(w.rollout.Object, t, "spec", "template"); err != nil {
			return fmt.Errorf("set .spec.template to rollout: %w", err)
		}
		if err := c.Update(ctx, w.rollout); err != nil {
			return fmt.Errorf("failed to update rollout: %w", err)
		}
		return nil
	}

	if err := c.Update(ctx, w.deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}
	return nil
}
