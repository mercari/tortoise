package vpa

import (
	"context"
	"fmt"
	"time"

	autoscaling "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/metrics"
)

type Service struct {
	c        versioned.Interface
	recorder record.EventRecorder
}

func New(c *rest.Config, recorder record.EventRecorder) (*Service, error) {
	cli, err := versioned.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	return &Service{c: cli, recorder: recorder}, nil
}

const TortoiseMonitorVPANamePrefix = "tortoise-monitor-"
const TortoiseUpdaterVPANamePrefix = "tortoise-updater-"
const TortoiseVPARecommenderName = "tortoise-controller"

func TortoiseMonitorVPAName(tortoiseName string) string {
	return TortoiseMonitorVPANamePrefix + tortoiseName
}

func TortoiseUpdaterVPAName(tortoiseName string) string {
	return TortoiseUpdaterVPANamePrefix + tortoiseName
}

func (c *Service) DeleteTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) error {
	if tortoise.Spec.DeletionPolicy == autoscalingv1beta3.DeletionPolicyNoDelete {
		return nil
	}

	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Get(ctx, TortoiseMonitorVPAName(tortoise.Name), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// already deleted
			return nil
		}
		return fmt.Errorf("failed to get vpa: %w", err)
	}

	// make sure it's created by tortoise
	if v, ok := vpa.Annotations[annotation.ManagedByTortoiseAnnotation]; !ok || v != "true" {
		// shouldn't reach here unless user manually remove the annotation.
		return nil
	}

	if err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Delete(ctx, vpa.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete vpa: %w", err)
	}
	return nil
}

func (c *Service) DeleteTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) error {
	if tortoise.Spec.DeletionPolicy == autoscalingv1beta3.DeletionPolicyNoDelete {
		return nil
	}

	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Get(ctx, TortoiseUpdaterVPAName(tortoise.Name), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// already deleted
			return nil
		}
		return fmt.Errorf("failed to get vpa: %w", err)
	}

	// make sure it's created by tortoise
	if v, ok := vpa.Annotations[annotation.ManagedByTortoiseAnnotation]; !ok || v != "true" {
		// shouldn't reach here unless user manually remove the annotation.
		return nil
	}

	if err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Delete(ctx, vpa.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete vpa: %w", err)
	}
	return nil
}

func (c *Service) CreateTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.VerticalPodAutoscaler, *autoscalingv1beta3.Tortoise, error) {
	auto := v1.UpdateModeAuto
	vpa := &v1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tortoise.Namespace,
			Name:      TortoiseUpdaterVPAName(tortoise.Name),
			Annotations: map[string]string{
				annotation.ManagedByTortoiseAnnotation: "true",
				annotation.TortoiseNameAnnotation:      tortoise.Name,
			},
		},
		Spec: v1.VerticalPodAutoscalerSpec{
			Recommenders: []*v1.VerticalPodAutoscalerRecommenderSelector{
				{
					Name: TortoiseVPARecommenderName, // This VPA is managed by Tortoise.
				},
			},
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.ScaleTargetRef.Name,
				APIVersion: "apps/v1",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: &auto,
			},
			ResourcePolicy: &v1.PodResourcePolicy{},
		},
	}
	crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
	for _, c := range tortoise.Spec.ResourcePolicy {
		crp = append(crp, v1.ContainerResourcePolicy{
			ContainerName: c.ContainerName,
			MinAllowed:    c.MinAllocatedResources,
		})
	}
	vpa.Spec.ResourcePolicy.ContainerPolicies = crp

	tortoise.Status.Targets.VerticalPodAutoscalers = append(tortoise.Status.Targets.VerticalPodAutoscalers, autoscalingv1beta3.TargetStatusVerticalPodAutoscaler{
		Name: vpa.Name,
		Role: autoscalingv1beta3.VerticalPodAutoscalerRoleUpdater,
	})
	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Create(ctx, vpa, metav1.CreateOptions{})
	if err != nil {
		return nil, tortoise, err
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, "VPACreated", fmt.Sprintf("Initialized a updator VPA %s/%s", vpa.Namespace, vpa.Name))

	return vpa, tortoise, nil
}

// UpdateVPAContainerResourcePolicy is update VPAs to have appropriate container policies based on tortoises' resource policy.
func (c *Service) UpdateVPAContainerResourcePolicy(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, vpa *v1.VerticalPodAutoscaler) (*v1.VerticalPodAutoscaler, *autoscalingv1beta3.Tortoise, error) {
	retVPA := &v1.VerticalPodAutoscaler{}
	var err error
	// we only want to record metric once in every reconcile loop.
	//metricsRecorded := false
	updateFn := func() error {
		crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
		for _, c := range tortoise.Spec.ResourcePolicy {
			crp = append(crp, v1.ContainerResourcePolicy{
				ContainerName: c.ContainerName,
				MinAllowed:    c.MinAllocatedResources,
			})
		}
		vpa.Spec.ResourcePolicy = &v1.PodResourcePolicy{ContainerPolicies: crp}
		retVPA, err = c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).UpdateStatus(ctx, vpa, metav1.UpdateOptions{})
		return err
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return retVPA, tortoise, fmt.Errorf("update VPA CRP status: %w", err)
	}

	return retVPA, tortoise, nil
}

func (c *Service) CreateTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.VerticalPodAutoscaler, *autoscalingv1beta3.Tortoise, error) {
	off := v1.UpdateModeOff
	vpa := &v1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tortoise.Namespace,
			Name:      TortoiseMonitorVPAName(tortoise.Name),
			Annotations: map[string]string{
				annotation.ManagedByTortoiseAnnotation: "true",
				annotation.TortoiseNameAnnotation:      tortoise.Name,
			},
		},
		Spec: v1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.ScaleTargetRef.Name,
				APIVersion: "apps/v1",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: &off,
			},
			ResourcePolicy: &v1.PodResourcePolicy{},
		},
	}
	crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
	for _, c := range tortoise.Spec.ResourcePolicy {
		crp = append(crp, v1.ContainerResourcePolicy{
			ContainerName: c.ContainerName,
			MinAllowed:    c.MinAllocatedResources,
		})
	}
	vpa.Spec.ResourcePolicy.ContainerPolicies = crp

	tortoise.Status.Targets.VerticalPodAutoscalers = append(tortoise.Status.Targets.VerticalPodAutoscalers, autoscalingv1beta3.TargetStatusVerticalPodAutoscaler{
		Name: vpa.Name,
		Role: autoscalingv1beta3.VerticalPodAutoscalerRoleMonitor,
	})

	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Create(ctx, vpa, metav1.CreateOptions{})
	if err != nil {
		return nil, tortoise, err
	}

	c.recorder.Event(tortoise, corev1.EventTypeNormal, "VPACreated", fmt.Sprintf("Initialized a monitor VPA %s/%s", vpa.Namespace, vpa.Name))

	return vpa, tortoise, nil
}

func (c *Service) UpdateVPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	retVPA := &v1.VerticalPodAutoscaler{}

	// we only want to record metric once in every reconcile loop.
	metricsRecorded := false
	updateFn := func() error {
		vpa, err := c.GetTortoiseUpdaterVPA(ctx, tortoise)
		if err != nil {
			return fmt.Errorf("get tortoise VPA: %w", err)
		}
		newRecommendations := make([]v1.RecommendedContainerResources, 0, len(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation))
		for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
			if !metricsRecorded {
				for resourcename, value := range r.RecommendedResource {
					if resourcename == corev1.ResourceCPU {
						metrics.ProposedCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.MilliValue()))
						if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
							// We don't want to record applied* metric when UpdateMode is Off.
							metrics.AppliedCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.MilliValue()))
						}
					}
					if resourcename == corev1.ResourceMemory {
						metrics.ProposedMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.Value()))
						if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
							// We don't want to record applied* metric when UpdateMode is Off.
							metrics.AppliedMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.Value()))
						}
					}
				}
				metricsRecorded = true
			}

			newRecommendations = append(newRecommendations, v1.RecommendedContainerResources{
				ContainerName:  r.ContainerName,
				Target:         r.RecommendedResource,
				LowerBound:     r.RecommendedResource,
				UpperBound:     r.RecommendedResource,
				UncappedTarget: r.RecommendedResource,
			})
		}
		if vpa.Status.Recommendation == nil {
			vpa.Status.Recommendation = &v1.RecommendedPodResources{}
		}
		vpa.Status.Recommendation.ContainerRecommendations = newRecommendations
		retVPA = vpa
		if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
			// don't update status if update mode is off. (= dryrun)
			return nil
		}
		retVPA, err = c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).UpdateStatus(ctx, vpa, metav1.UpdateOptions{})
		return err
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return retVPA, fmt.Errorf("update VPA status: %w", err)
	}

	if tortoise.Spec.UpdateMode != autoscalingv1beta3.UpdateModeOff {
		c.recorder.Event(tortoise, corev1.EventTypeNormal, "VPAUpdated", fmt.Sprintf("VPA %s/%s is updated by the recommendation. The Pods should also be updated with new resources soon by VPA if needed", retVPA.Namespace, retVPA.Name))
	}

	return retVPA, nil
}

func (c *Service) GetTortoiseUpdaterVPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.VerticalPodAutoscaler, error) {
	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Get(ctx, TortoiseUpdaterVPAName(tortoise.Name), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get updater vpa on tortoise: %w", err)
	}
	return vpa, nil
}

func (c *Service) GetTortoiseMonitorVPA(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise) (*v1.VerticalPodAutoscaler, bool, error) {
	vpa, err := c.c.AutoscalingV1().VerticalPodAutoscalers(tortoise.Namespace).Get(ctx, TortoiseMonitorVPAName(tortoise.Name), metav1.GetOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("failed to get updater vpa on tortoise: %w", err)
	}

	for _, c := range vpa.Status.Conditions {
		if c.Type == v1.RecommendationProvided && c.Status == corev1.ConditionTrue {
			return vpa, true, nil
		}
	}

	return vpa, false, nil
}

func SetAllVerticalContainerResourcePhaseWorking(tortoise *autoscalingv1beta3.Tortoise, now time.Time) *autoscalingv1beta3.Tortoise {
	verticalResourceAndContainer := sets.New[resourceNameAndContainerName]()
	for _, p := range tortoise.Status.AutoscalingPolicy {
		for rn, ap := range p.Policy {
			if ap == autoscalingv1beta3.AutoscalingTypeVertical {
				verticalResourceAndContainer.Insert(resourceNameAndContainerName{rn, p.ContainerName})
			}
		}
	}

	found := false
	for _, d := range verticalResourceAndContainer.UnsortedList() {
		for i, p := range tortoise.Status.ContainerResourcePhases {
			if p.ContainerName == d.containerName {
				tortoise.Status.ContainerResourcePhases[i].ResourcePhases[d.rn] = autoscalingv1beta3.ResourcePhase{
					Phase:              autoscalingv1beta3.ContainerResourcePhaseWorking,
					LastTransitionTime: metav1.NewTime(now),
				}
				found = true
				break
			}
		}
		if !found {
			tortoise.Status.ContainerResourcePhases = append(tortoise.Status.ContainerResourcePhases, autoscalingv1beta3.ContainerResourcePhases{
				ContainerName: d.containerName,
				ResourcePhases: map[corev1.ResourceName]autoscalingv1beta3.ResourcePhase{
					d.rn: {
						Phase:              autoscalingv1beta3.ContainerResourcePhaseWorking,
						LastTransitionTime: metav1.NewTime(now),
					},
				},
			})
		}
	}

	return tortoise
}

type resourceNameAndContainerName struct {
	rn            corev1.ResourceName
	containerName string
}
