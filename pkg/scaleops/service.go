package scaleops

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1beta3 "github.com/mercari/tortoise/api/v1beta3"
)

const (
	// ScaleOps API group and version
	ScaleOpsAPIGroup   = "analysis.scaleops.sh"
	ScaleOpsAPIVersion = "v1alpha1"
)

// Service provides ScaleOps CRD detection functionality
type Service struct {
	client     client.Client
	crdEnabled bool
}

// New creates a new ScaleOps service
func New(c client.Client) *Service {
	crdEnabled := checkScaleOpsCRDsInstalled(c)
	if !crdEnabled {
		// Log once at startup that ScaleOps CRDs are not installed
		logger := log.Log.WithName("scaleops-service")
		logger.Info("ScaleOps CRDs are not installed in this cluster, ScaleOps detection disabled")
	}
	return &Service{
		client:     c,
		crdEnabled: crdEnabled,
	}
}

// IsScaleOpsManaged checks if a workload is managed by ScaleOps using two-level detection
// Returns: (isManaged bool, reason string, error)
// Priority: Recommendation (workload-level) > AutomatedNamespace (namespace-level)
func (s *Service) IsScaleOpsManaged(ctx context.Context, tortoise *v1beta3.Tortoise) (bool, string, error) {
	if !s.crdEnabled {
		return false, "", nil
	}

	logger := log.FromContext(ctx)

	// Level 1: Check Recommendation (workload-level - most specific)
	managed, reason, err := s.checkRecommendation(ctx, tortoise)
	if err != nil {
		return false, "", err
	}
	if managed {
		logger.V(4).Info("workload is managed by ScaleOps", "reason", reason)
		return true, reason, nil
	}
	if reason == "WorkloadOptedOut" {
		// Workload explicitly opted out of namespace-level automation
		logger.V(4).Info("workload explicitly opted out of ScaleOps automation")
		return false, "", nil
	}

	// Level 2: Check AutomatedNamespace (namespace-level - less specific)
	managed, reason, err = s.checkAutomatedNamespace(ctx, tortoise)
	if err != nil {
		return false, "", err
	}
	if managed {
		logger.V(4).Info("namespace is managed by ScaleOps", "reason", reason)
		return true, reason, nil
	}

	return false, "", nil
}

// checkRecommendation checks if workload has a Recommendation CRD with automation enabled
func (s *Service) checkRecommendation(ctx context.Context, tortoise *v1beta3.Tortoise) (bool, string, error) {
	logger := log.FromContext(ctx)

	// Construct recommendation name: {kind}-{name}
	kind := strings.ToLower(tortoise.Spec.TargetRefs.ScaleTargetRef.Kind)
	name := tortoise.Spec.TargetRefs.ScaleTargetRef.Name
	recName := fmt.Sprintf("%s-%s", kind, name)

	// Use Unstructured to fetch the resource
	rec := &unstructured.Unstructured{}
	rec.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ScaleOpsAPIGroup,
		Version: ScaleOpsAPIVersion,
		Kind:    "Recommendation",
	})

	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: tortoise.Namespace,
		Name:      recName,
	}, rec)

	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("no Recommendation found",
				"namespace", tortoise.Namespace,
				"recommendation", recName)
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to get Recommendation %s/%s: %w", tortoise.Namespace, recName, err)
	}

	// Extract spec fields from unstructured map
	spec, found, err := unstructured.NestedMap(rec.Object, "spec")
	if err != nil {
		return false, "", fmt.Errorf("failed to extract spec from Recommendation: %w", err)
	}
	if !found {
		logger.V(4).Info("Recommendation has no spec field", "recommendation", recName)
		return false, "", nil
	}

	// Check if workload is explicitly excluded from automation
	automationExcluded, _, _ := unstructured.NestedBool(spec, "automationExcluded")
	if automationExcluded {
		logger.Info("workload explicitly opted out of ScaleOps automation",
			"namespace", tortoise.Namespace,
			"recommendation", recName)
		return false, "WorkloadOptedOut", nil
	}

	// Check optimize field (rightsizing)
	optimize, _, _ := unstructured.NestedBool(spec, "optimize")

	// Check scaleOutOptimize field (replicas/HPA)
	scaleOutOptimize, _, _ := unstructured.NestedBool(spec, "scaleOutOptimize")

	if optimize || scaleOutOptimize {
		logger.Info("workload managed by ScaleOps at workload level",
			"namespace", tortoise.Namespace,
			"recommendation", recName,
			"rightsizing", optimize,
			"replicas", scaleOutOptimize)
		return true, fmt.Sprintf("ScaleOpsManaged(Recommendation:%s)", recName), nil
	}

	// Recommendation exists but automation is disabled (implicit opt-out)
	logger.V(4).Info("Recommendation exists but automation is disabled",
		"recommendation", recName)
	return false, "WorkloadOptedOut", nil
}

// checkAutomatedNamespace checks if namespace has an AutomatedNamespace CRD with automation enabled
func (s *Service) checkAutomatedNamespace(ctx context.Context, tortoise *v1beta3.Tortoise) (bool, string, error) {
	logger := log.FromContext(ctx)

	// AutomatedNamespace is named the same as the namespace
	ans := &unstructured.Unstructured{}
	ans.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ScaleOpsAPIGroup,
		Version: ScaleOpsAPIVersion,
		Kind:    "AutomatedNamespace",
	})

	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: tortoise.Namespace,
		Name:      tortoise.Namespace,
	}, ans)

	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("no AutomatedNamespace found", "namespace", tortoise.Namespace)
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to get AutomatedNamespace %s: %w", tortoise.Namespace, err)
	}

	// Extract spec fields
	spec, found, err := unstructured.NestedMap(ans.Object, "spec")
	if err != nil {
		return false, "", fmt.Errorf("failed to extract spec from AutomatedNamespace: %w", err)
	}
	if !found {
		logger.V(4).Info("AutomatedNamespace has no spec field", "namespace", tortoise.Namespace)
		return false, "", nil
	}

	// Check rightsizeOptimize (new field)
	rightsizeOptimize, _, _ := unstructured.NestedBool(spec, "rightsizeOptimize")

	// Check optimize (legacy field, still supported by ScaleOps)
	optimize, _, _ := unstructured.NestedBool(spec, "optimize")

	// Check replicasOptimize
	replicasOptimize, _, _ := unstructured.NestedBool(spec, "replicasOptimize")

	rightsizeOn := rightsizeOptimize || optimize
	replicasOn := replicasOptimize

	if rightsizeOn || replicasOn {
		logger.Info("namespace managed by ScaleOps at namespace level",
			"namespace", tortoise.Namespace,
			"rightsizing", rightsizeOn,
			"replicas", replicasOn)
		return true, "ScaleOpsManaged(AutomatedNamespace)", nil
	}

	return false, "", nil
}

// checkScaleOpsCRDsInstalled checks if ScaleOps CRDs are installed in the cluster
func checkScaleOpsCRDsInstalled(c client.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Try to list AutomatedNamespaces to check if CRD exists
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ScaleOpsAPIGroup,
		Version: ScaleOpsAPIVersion,
		Kind:    "AutomatedNamespaceList",
	})

	err := c.List(ctx, list, &client.ListOptions{Limit: 1})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD not installed
			return false
		}
		// Other errors (permission denied, etc.) - assume CRDs exist
		return true
	}

	return true
}
