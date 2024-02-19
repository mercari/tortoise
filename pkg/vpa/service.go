package vpa

import (
	"context"
	"fmt"
	"reflect"
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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/annotation"
	"github.com/mercari/tortoise/pkg/event"
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

const tortoiseMonitorVPANamePrefix = "tortoise-monitor-"
const tortoiseUpdaterVPANamePrefix = "tortoise-updater-"
const tortoiseVPARecommenderName = "tortoise-controller"

func TortoiseMonitorVPAName(tortoiseName string) string {
	return tortoiseMonitorVPANamePrefix + tortoiseName
}

func TortoiseUpdaterVPAName(tortoiseName string) string {
	return tortoiseUpdaterVPANamePrefix + tortoiseName
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
					Name: tortoiseVPARecommenderName, // This VPA is managed by Tortoise.
				},
			},
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       tortoise.Spec.TargetRefs.ScaleTargetRef.Name,
				APIVersion: "apps/v1",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: ptr.To(v1.UpdateModeInitial),
			},
			ResourcePolicy: &v1.PodResourcePolicy{},
		},
	}
	crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
	for _, c := range tortoise.Spec.ResourcePolicy {
		if c.MinAllocatedResources == nil {
			continue
		}
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

	c.recorder.Event(tortoise, corev1.EventTypeNormal, event.VPACreated, fmt.Sprintf("Initialized a updator VPA %s/%s", vpa.Namespace, vpa.Name))

	return vpa, tortoise, nil
}

// UpdateVPAContainerResourcePolicy is update VPAs to have appropriate container policies based on tortoises' resource policy.
func (c *Service) UpdateVPAContainerResourcePolicy(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, vpa *v1.VerticalPodAutoscaler) (*v1.VerticalPodAutoscaler, error) {
	retVPA := &v1.VerticalPodAutoscaler{}
	var err error

	updateFn := func() error {
		crp := make([]v1.ContainerResourcePolicy, 0, len(tortoise.Spec.ResourcePolicy))
		for _, c := range tortoise.Spec.ResourcePolicy {
			crp = append(crp, v1.ContainerResourcePolicy{
				ContainerName: c.ContainerName,
				MinAllowed:    c.MinAllocatedResources,
			})
		}
		vpa.Spec.ResourcePolicy = &v1.PodResourcePolicy{ContainerPolicies: crp}
		retVPA, err = c.c.AutoscalingV1().VerticalPodAutoscalers(vpa.Namespace).Update(ctx, vpa, metav1.UpdateOptions{})
		return err
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return retVPA, fmt.Errorf("update VPA ContainerResourcePolicy status: %w", err)
	}

	return retVPA, nil
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
		if c.MinAllocatedResources == nil {
			continue
		}
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

	c.recorder.Event(tortoise, corev1.EventTypeNormal, event.VPACreated, fmt.Sprintf("Initialized a monitor VPA %s/%s", vpa.Namespace, vpa.Name))

	return vpa, tortoise, nil
}

// UpdateVPAFromTortoiseRecommendation updates VPA with the recommendation from Tortoise.
// In the second return value, it returns true if the Pods should be updated with new resources.
func (c *Service) UpdateVPAFromTortoiseRecommendation(ctx context.Context, tortoise *autoscalingv1beta3.Tortoise, replica int32, now time.Time) (*v1.VerticalPodAutoscaler, *autoscalingv1beta3.Tortoise, bool, error) {
	newVPA := &v1.VerticalPodAutoscaler{}
	// At the end of this function, this will be true:
	// - if UpdateMode is Off, this will be always false.
	// - if UpdateMode is Auto, this will be true if any of the recommended resources is increased,
	//   OR, if all the recommended resources is decreased and it's been a while (1h) after the last update.
	//   (We don't want to update the Pod too frequently if it's only for scaling down.)
	podShouldBeUpdatedWithNewResource := false

	// we only want to record metric once in every reconcile loop.
	metricsRecorded := false
	updateFn := func() error {
		oldVPA, err := c.GetTortoiseUpdaterVPA(ctx, tortoise)
		if err != nil {
			return fmt.Errorf("get tortoise VPA: %w", err)
		}
		newVPA = oldVPA.DeepCopy()
		newRecommendations := make([]v1.RecommendedContainerResources, 0, len(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation))
		newRequests := make([]autoscalingv1beta3.ContainerResourceRequests, 0, len(tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation))
		for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
			if !metricsRecorded {
				// only record metrics once in every reconcile loop.
				//
				// We only records proposed* metrics and don't record applied* metrics here.
				for resourcename, value := range r.RecommendedResource {
					if resourcename == corev1.ResourceCPU {
						metrics.ProposedCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.MilliValue()))
					}
					if resourcename == corev1.ResourceMemory {
						metrics.ProposedMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.Value()))
					}
				}
			}

			newRecommendations = append(newRecommendations, v1.RecommendedContainerResources{
				ContainerName:  r.ContainerName,
				Target:         r.RecommendedResource,
				LowerBound:     r.RecommendedResource,
				UpperBound:     r.RecommendedResource,
				UncappedTarget: r.RecommendedResource,
			})
			newRequests = append(newRequests, autoscalingv1beta3.ContainerResourceRequests{
				ContainerName: r.ContainerName,
				Resource:      r.RecommendedResource,
			})
		}
		metricsRecorded = true
		if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
			if oldVPA.Status.Recommendation == nil {
				// nothing to do.
				return nil
			}

			// remove recommendation if UpdateMode is Off so that VPA won't update the Pod.
			newRecommendations = nil
		}

		if newVPA.Status.Recommendation == nil {
			// It's the first time to update VPA.
			newVPA.Status.Recommendation = &v1.RecommendedPodResources{}
			newVPA.Status.Conditions = nil // make sure it's nil
		}

		newVPA.Spec.UpdatePolicy = &v1.PodUpdatePolicy{
			UpdateMode: ptr.To(v1.UpdateModeInitial),
		}
		newVPA.Status.Recommendation.ContainerRecommendations = newRecommendations

		if oldVPA.Status.Recommendation != nil && reflect.DeepEqual(newRecommendations, oldVPA.Status.Recommendation.ContainerRecommendations) {
			// If the recommendation is not changed at all, we don't need to update VPA and Pods.
			podShouldBeUpdatedWithNewResource = false
			newVPA = oldVPA
			return nil
		}

		increased := recommendationIncreaseAnyResource(oldVPA, newVPA)
		for _, v := range newVPA.Status.Conditions {
			if v.Type == v1.RecommendationProvided && v.Status == corev1.ConditionTrue {
				if v.LastTransitionTime.Add(time.Hour).After(now) && !increased {
					// if all the recommended resources is decreased and it's NOT yet been 1h after the last update,
					// we don't want to update the Pod too frequently.
					log.FromContext(ctx).Info("Skip updating VPA status because it's been less than 1h since the last update", "tortoise", tortoise.Name, "namespace", tortoise.Namespace, "vpa", newVPA.Name)
					newVPA = oldVPA
					podShouldBeUpdatedWithNewResource = false
					return nil
				}
			}
		}

		// The recommendation will be applied to VPA and the deployment will be restarted with the new resources.
		// Update the request recorded in the status, which will be used in the next reconcile loop.
		tortoise.Status.Conditions.ContainerResourceRequests = newRequests

		newVPA.Status.Conditions = []v1.VerticalPodAutoscalerCondition{
			{
				Type:               v1.RecommendationProvided,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now),
				Message:            fmt.Sprintf("The recommendation is provided from Tortoise(%v)", tortoise.Name),
			},
		}
		if tortoise.Spec.UpdateMode == autoscalingv1beta3.UpdateModeOff {
			newVPA.Status.Conditions = []v1.VerticalPodAutoscalerCondition{
				{
					Type:               v1.RecommendationProvided,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(now),
					Message:            fmt.Sprintf("The recommendation is not provided from Tortoise(%v) because it's Off mode", tortoise.Name),
				},
			}
		}
		podShouldBeUpdatedWithNewResource = true

		// If VPA CRD in the cluster hasn't got the status subresource yet, this will update the status as well.
		newVPA2, err := c.c.AutoscalingV1().VerticalPodAutoscalers(newVPA.Namespace).Update(ctx, newVPA, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update VPA MinReplicas (%s/%s): %w", newVPA.Namespace, newVPA.Name, err)
		}
		newVPA2.Status = newVPA.Status

		// Then, we update VPA status (Recommendation).
		newVPA3, err := c.c.AutoscalingV1().VerticalPodAutoscalers(newVPA.Namespace).UpdateStatus(ctx, newVPA2, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Ignore it. Probably it's because VPA CRD hasn't got the status subresource yet.
				newVPA = newVPA2
				return nil
			}
			return fmt.Errorf("update VPA (%s/%s) status: %w", newVPA.Namespace, newVPA.Name, err)
		}
		newVPA = newVPA3

		return nil
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, updateFn); err != nil {
		return newVPA, tortoise, podShouldBeUpdatedWithNewResource, fmt.Errorf("update VPA status: %w", err)
	}

	if tortoise.Spec.UpdateMode != autoscalingv1beta3.UpdateModeOff && podShouldBeUpdatedWithNewResource {
		c.recorder.Event(tortoise, corev1.EventTypeNormal, event.VPAUpdated, fmt.Sprintf("VPA %s/%s is updated by the recommendation. The Pods should also be updated with new resources soon", newVPA.Namespace, newVPA.Name))
		for _, r := range tortoise.Status.Recommendations.Vertical.ContainerResourceRecommendation {
			// only record metrics once in every reconcile loop.
			for resourcename, value := range r.RecommendedResource {
				if resourcename == corev1.ResourceCPU {
					// We don't want to record applied* metric when UpdateMode is Off.
					metrics.AppliedCPURequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.MilliValue()))
				}
				if resourcename == corev1.ResourceMemory {
					metrics.AppliedMemoryRequest.WithLabelValues(tortoise.Name, tortoise.Namespace, r.ContainerName, tortoise.Spec.TargetRefs.ScaleTargetRef.Name, tortoise.Spec.TargetRefs.ScaleTargetRef.Kind).Set(float64(value.Value()))
				}
			}
		}
	}

	return newVPA, tortoise, podShouldBeUpdatedWithNewResource, nil
}

func recommendationIncreaseAnyResource(oldVPA, newVPA *v1.VerticalPodAutoscaler) bool {
	if oldVPA.Status.Recommendation == nil {
		// if oldVPA doesn't have recommendation, it means it's the first time to update VPA.
		return true
	}
	if newVPA.Status.Recommendation == nil {
		// if newVPA doesn't have recommendation, it means we're going to remove the recommendation.
		return true
	}

	for _, new := range newVPA.Status.Recommendation.ContainerRecommendations {
		found := false
		for _, old := range oldVPA.Status.Recommendation.ContainerRecommendations {
			if old.ContainerName != new.ContainerName {
				continue
			}
			found = true
			if old.Target.Cpu().Cmp(*new.Target.Cpu()) < 0 || old.Target.Memory().Cmp(*new.Target.Memory()) < 0 {
				return true
			}
		}
		if !found {
			// if the container is not found in oldVPA, it means it's the first time to update VPA with that container's recommendation.
			return true
		}
	}

	return false
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

	return vpa, isMonitorVPAReady(vpa, tortoise), nil
}

func isMonitorVPAReady(vpa *v1.VerticalPodAutoscaler, tortoise *autoscalingv1beta3.Tortoise) bool {
	provided := false
	for _, c := range vpa.Status.Conditions {
		if c.Type == v1.RecommendationProvided && c.Status == corev1.ConditionTrue {
			provided = true
		}
	}
	if !provided {
		return false
	}

	// Check if VPA has the recommendation for all the containers registered in the tortoise.
	containerInTortoise := sets.New[string]()
	for _, p := range tortoise.Status.AutoscalingPolicy {
		containerInTortoise.Insert(p.ContainerName)
	}

	containerInVPA := sets.New[string]()
	for _, c := range vpa.Status.Recommendation.ContainerRecommendations {
		containerInVPA.Insert(c.ContainerName)
		if c.Target.Cpu().IsZero() || c.Target.Memory().IsZero() {
			// something wrong with the recommendation.
			return false
		}
	}

	return containerInTortoise.Equal(containerInVPA)
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
