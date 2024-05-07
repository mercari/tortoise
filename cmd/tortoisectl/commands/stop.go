package commands

import (
	"fmt"
	"os"
	"path/filepath"

	autoscalingv1beta3 "github.com/mercari/tortoise/api/v1beta3"
	"github.com/mercari/tortoise/pkg/deployment"
	"github.com/mercari/tortoise/pkg/pod"
	"github.com/mercari/tortoise/pkg/stoper"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var stopCmd = &cobra.Command{
	Use:   "stop tortoise1 tortoise2...",
	Short: "stop tortoise(s) safely",
	Long: `
stop is the command to temporarily turn off tortoise(s) easily and safely.

It's intended to be used when your application is facing issues that might be caused by tortoise.
Specifically, it changes the tortoise updateMode to "Off" and restarts the deployment to bring the pods back to the original resource requests.

Also, with the --no-lowering-resources flag, it patches the deployment directly
so that changing tortoise to Off won't result in lowering the resource request(s), damaging the service.
e.g., if the Deployment declares 1 CPU request, and the current Pods' request is 2 CPU mutated by Tortoise,
it'd patch the deployment to 2 CPU request to prevent a possible negative impact on the service. 
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// validation
		if stopAll {
			if len(args) != 0 {
				return fmt.Errorf("tortoise name shouldn't be specified because of --all flag")
			}
		} else {
			if stopNamespace == "" {
				return fmt.Errorf("namespace must be specified")
			}
			if len(args) == 0 {
				return fmt.Errorf("tortoise name must be specified")
			}
		}

		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return fmt.Errorf("failed to build config: %v", err)
		}

		client, err := client.New(config, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}

		recorder := record.NewBroadcaster().NewRecorder(scheme, corev1.EventSource{Component: "tortoisectl"})
		deploymentService := deployment.New(client, "", "", recorder)
		podService, err := pod.New(map[string]int64{}, "", nil, nil)
		if err != nil {
			return fmt.Errorf("failed to create pod service: %v", err)
		}

		stoperService := stoper.New(client, deploymentService, podService)

		opts := []stoper.StoprOption{}
		if noLoweringResources {
			opts = append(opts, stoper.NoLoweringResource)
		}

		err = stoperService.Stop(cmd.Context(), args, stopNamespace, stopAll, os.Stdout, opts...)
		if err != nil {
			return fmt.Errorf("failed to stop tortoise(s): %v", err)
		}

		return nil
	},
}

var (
	// namespace to stop tortoise(s) in
	stopNamespace string
	// stop all tortoises in the specified namespace, or in all namespaces if no namespace is specified.
	stopAll bool
	// Stop tortoise without lowering resource requests.
	// If this flag is specified and the current Deployment's resource request(s) is lower than the current Pods' request mutated by Tortoise,
	// this CLI patches the deployment so that changing tortoise to Off won't result in lowering the resource request(s), damaging the service.
	noLoweringResources bool

	// Path to KUBECONFIG
	kubeconfig string

	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(autoscalingv1beta3.AddToScheme(scheme))

	rootCmd.AddCommand(stopCmd)

	if home := homedir.HomeDir(); home != "" {
		stopCmd.Flags().StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		stopCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}

	stopCmd.Flags().StringVarP(&stopNamespace, "namespace", "n", "", "namespace to stop tortoise(s) in")
	stopCmd.Flags().BoolVarP(&stopAll, "all", "A", false, "stop all tortoises in the specified namespace, or in all namespaces if no namespace is specified.")
	stopCmd.Flags().BoolVar(&noLoweringResources, "no-lowering-resources", false, `Stop tortoise without lowering resource requests. 
	If this flag is specified and the current Deployment's resource request(s) is lower than the current Pods' request mutated by Tortoise,
	this CLI patches the deployment so that changing tortoise to Off won't result in lowering the resource request(s), damaging the service.`)
}
