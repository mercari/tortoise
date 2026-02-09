package scaleops

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
