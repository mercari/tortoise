package stoper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/kyokomi/emoji/v2"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/pod"
)

// Stopr is the struct for stopping tortoise safely.
type Stopr struct {
	c client.Client

	deploymentService *deployment.Service
	podService        *pod.Service
}

func New(c client.Client, ds *deployment.Service, ps *pod.Service) *Stopr {
	return &Stopr{
		c:                 c,
		deploymentService: ds,
		podService:        ps,
	}
}

type StoprOption string

var (
	NoLoweringResource StoprOption = "NoLoweringResource"
)

func (s *Stopr) Stop(ctx context.Context, tortoiseNames []string, namespace string, all bool, writer io.Writer, opts ...StoprOption) error {
	// It assumes the validation is already done in the CLI layer.

	targets := []types.NamespacedName{}
	if all {
		tortoises := &v1beta3.TortoiseList{}
		opt := &client.ListOptions{}
		if namespace != "" {
			// stop all tortoises in the namespace
			opt.Namespace = namespace
		}

		if err := s.c.List(ctx, tortoises, opt); err != nil {
			return fmt.Errorf("failed to list tortoises: %w", err)
		}

		for _, t := range tortoises.Items {
			targets = append(targets, types.NamespacedName{Name: t.Name, Namespace: t.Namespace})
		}
	} else {
		for _, name := range tortoiseNames {
			targets = append(targets, types.NamespacedName{Name: name, Namespace: namespace})
		}
	}

	var finalerr error
	for _, target := range targets {
		write(writer, fmt.Sprintf("\n%s stopping your tortoise %s ... ", emoji.Sprint(":turtle:"), &target))

		// 1. Stop Tortoise.
		tortoise, err := s.stopOne(ctx, target)
		if err != nil {
			if errors.Is(err, errTortoiseAlreadyStopped) {
				write(writer, fmt.Sprintf("this tortoise is already stopped %s\n", emoji.Sprint(":sleeping:")))
				continue
			}
			finalerr = errors.Join(finalerr, err)
			write(writer, fmt.Sprintf("failed to stop your tortoise %s.\nError: %v\n", emoji.Sprint(":face_with_spiral_eyes:"), err))
			continue
		}
		write(writer, fmt.Sprintf("Done %s\n", emoji.Sprint(":sleeping:")))

		// 2. Get the target deployment.
		dp, err := s.deploymentService.GetDeploymentOnTortoise(ctx, tortoise)
		if err != nil {
			finalerr = errors.Join(finalerr, err)
			write(writer, fmt.Sprintf("%s failed to get deployment on your tortoise %s.\nError: %v\n", emoji.Sprint(":face_with_spiral_eyes:"), &target, err))
			continue
		}

		// 3. [when NoLoweringResource is true] Patch the deployment to keep the resource requests high.
		if containsOption(opts, NoLoweringResource) {
			write(writer, fmt.Sprintf("%s patching your deployment to keep the resource requests high ... ", emoji.Sprint(":hammer_and_wrench:")))
			updated, err := s.patchDeploymentToKeepResources(ctx, dp, tortoise)
			if err != nil {
				finalerr = errors.Join(finalerr, err)
				write(writer, fmt.Sprintf("%s failed to patch your deployment %s.\nError: %v\n", emoji.Sprint(":face_with_spiral_eyes:"), &target, err))
				continue
			}

			write(writer, fmt.Sprintf("Done %s\n", emoji.Sprint(":hammer_and_wrench:")))

			if updated {
				// If the deployment is updated, we don't need to restart the deployment.
				continue
			}
		}

		// 4. Restart the deployment to get back the original resource requests.
		write(writer, fmt.Sprintf("%s restarting your deployment to get back the original resource ... ", emoji.Sprint(":arrows_counterclockwise:")))
		if err := s.deploymentService.RolloutRestart(ctx, dp, tortoise, time.Now()); err != nil {
			finalerr = errors.Join(finalerr, err)
			write(writer, fmt.Sprintf("%s failed to restart your deployment %s.\nError: %v\n", emoji.Sprint(":face_with_spiral_eyes:"), &target, err))
			continue
		}
		write(writer, fmt.Sprintf("Done, your Pods should get the original resources soon %s\n", emoji.Sprint(":muscle:")))
	}

	return finalerr
}

func write(writer io.Writer, msg string) {
	//nolint:errcheck // intentionally ignore the error because it's not critical
	writer.Write([]byte(msg))
}

func containsOption(opts []StoprOption, opt StoprOption) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

// patchDeploymentToKeepResources patches the deployment to keep the resource requests high.
// The first return value indicates whether the deployment is updated or not.
func (s *Stopr) patchDeploymentToKeepResources(ctx context.Context, dp *v1.Deployment, tortoise *v1beta3.Tortoise) (bool, error) {
	originalDP := dp.DeepCopy()

	// Set to Auto because ModifyPodSpecResource doesn't change anything if it's set to Off.
	tortoise.Spec.UpdateMode = v1beta3.UpdateModeAuto
	s.podService.ModifyPodTemplateResource(&dp.Spec.Template, tortoise, pod.NoScaleDown)

	tortoise.Spec.UpdateMode = v1beta3.UpdateModeOff
	// If not updated, early return
	if reflect.DeepEqual(originalDP.Spec.Template.Spec.Containers, dp.Spec.Template.Spec.Containers) {
		return false, nil
	}

	if err := s.c.Update(ctx, dp); err != nil {
		return false, fmt.Errorf("failed to update deployment: %w", err)
	}

	return true, nil
}

var errTortoiseAlreadyStopped = errors.New("tortoise is already stopped")

// stopOne disables the tortoise.
func (s *Stopr) stopOne(ctx context.Context, target types.NamespacedName) (*v1beta3.Tortoise, error) {
	t := &v1beta3.Tortoise{}
	if err := s.c.Get(ctx, target, t); err != nil {
		return nil, fmt.Errorf("failed to get tortoise: %w", err)
	}

	if t.Spec.UpdateMode == v1beta3.UpdateModeOff {
		return t, errTortoiseAlreadyStopped
	}

	t.Spec.UpdateMode = v1beta3.UpdateModeOff

	if err := s.c.Update(ctx, t); err != nil {
		return nil, fmt.Errorf("failed to update tortoise: %w", err)
	}

	return t, nil
}
