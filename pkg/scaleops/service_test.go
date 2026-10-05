package scaleops

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/mercari/tortoise/api/v1beta3"
)

func TestService_IsScaleOpsManaged_CRDNotEnabled(t *testing.T) {
	// Test that when CRD is not enabled, IsScaleOpsManaged returns false
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	s := &Service{
		client:     fakeClient,
		crdEnabled: false,
	}

	tortoise := &v1beta3.Tortoise{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta3.TortoiseSpec{
			TargetRefs: v1beta3.TargetRefs{
				ScaleTargetRef: v1beta3.CrossVersionObjectReference{
					Kind: "Deployment",
					Name: "test-deployment",
				},
			},
		},
	}

	managed, reason, err := s.IsScaleOpsManaged(context.TODO(), tortoise)
	if err != nil {
		t.Errorf("IsScaleOpsManaged() unexpected error = %v", err)
	}
	if managed {
		t.Errorf("IsScaleOpsManaged() gotManaged = true, want false when CRD not enabled")
	}
	if reason != "" {
		t.Errorf("IsScaleOpsManaged() gotReason = %v, want empty when CRD not enabled", reason)
	}
}

// Note: Integration tests with actual CRD objects should be added to e2e tests
// as the fake client has limitations with Unstructured objects.
// The main logic paths are tested through the controller integration tests.

func newScaleOpsObject(kind, namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	o := &unstructured.Unstructured{Object: map[string]interface{}{"spec": spec}}
	o.SetGroupVersionKind(schema.GroupVersionKind{Group: ScaleOpsAPIGroup, Version: ScaleOpsAPIVersion, Kind: kind})
	o.SetNamespace(namespace)
	o.SetName(name)
	return o
}

func TestService_IsScaleOpsManaged_Recommendation(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		objs        []client.Object
		wantManaged bool
		wantReason  string
	}{
		{
			name:        "Deployment managed at workload level",
			kind:        "Deployment",
			objs:        []client.Object{newScaleOpsObject("Recommendation", "default", "deployment-app", map[string]interface{}{"optimize": true})},
			wantManaged: true,
			wantReason:  "ScaleOpsManagedWorkload:app",
		},
		{
			name:        "Rollout managed at workload level (Recommendation named after the Rollout's family)",
			kind:        "Rollout",
			objs:        []client.Object{newScaleOpsObject("Recommendation", "default", "family-scaleops-rollout-app", map[string]interface{}{"scaleOutOptimize": true})},
			wantManaged: true,
			wantReason:  "ScaleOpsManagedWorkload:app",
		},
		{
			name: "Rollout opted out at workload level wins over the namespace-level automation",
			kind: "Rollout",
			objs: []client.Object{
				newScaleOpsObject("Recommendation", "default", "family-scaleops-rollout-app", map[string]interface{}{"optimize": true, "automationExcluded": true}),
				newScaleOpsObject("AutomatedNamespace", "default", "default", map[string]interface{}{"rightsizeOptimize": true}),
			},
			wantManaged: false,
		},
		{
			name:        "Rollout without Recommendation falls back to the namespace level",
			kind:        "Rollout",
			objs:        []client.Object{newScaleOpsObject("AutomatedNamespace", "default", "default", map[string]interface{}{"rightsizeOptimize": true})},
			wantManaged: true,
			wantReason:  "ScaleOpsManagedNamespace",
		},
		{
			name: "Rollout isn't matched by the Recommendation of the Deployment with the same name",
			kind: "Rollout",
			objs: []client.Object{newScaleOpsObject("Recommendation", "default", "deployment-app", map[string]interface{}{"optimize": true})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				client:     fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tt.objs...).Build(),
				crdEnabled: true,
			}
			tortoise := &v1beta3.Tortoise{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test"},
				Spec: v1beta3.TortoiseSpec{
					TargetRefs: v1beta3.TargetRefs{
						ScaleTargetRef: v1beta3.CrossVersionObjectReference{Kind: tt.kind, Name: "app"},
					},
				},
			}

			managed, reason, err := s.IsScaleOpsManaged(context.TODO(), tortoise)
			if err != nil {
				t.Fatalf("IsScaleOpsManaged() unexpected error = %v", err)
			}
			if managed != tt.wantManaged {
				t.Errorf("IsScaleOpsManaged() gotManaged = %v, want %v", managed, tt.wantManaged)
			}
			if reason != tt.wantReason {
				t.Errorf("IsScaleOpsManaged() gotReason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
