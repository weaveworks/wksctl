package main

import (
	"context"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	wks "github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/wksctl"
	baremetalv1 "github.com/weaveworks/wksctl/pkg/baremetal/v1alpha3"
	machineutil "github.com/weaveworks/wksctl/pkg/cluster/machine"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var options struct {
	verbose bool
}

func main() {
	rootCmd.PersistentFlags().BoolVarP(&options.verbose, "verbose", "v", false, "Enable verbose output")
	Execute()
}

func initializeControllerNamespace(c client.Client) (string, error) {
	controllerNamespace, err := machineutil.GetKubernetesNamespaceFromMachines(context.Background(), c)
	if err != nil {
		return "", err
	}
	return controllerNamespace, nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "controller",
	Short: "WKS cluster controller",
	Long: `WKS cluster controller.

It is responsible for monitoring the Kubernetes cluster within which it runs,
and ensuring its actual state always converges towards its description.

The description of a cluster is typically done using custom resource
definitions (CRDs), e.g. Cluster and Machine objects.

When these objects are stored in Kubernetes API server's storage backend, this
WKS controller can then monitor them.

Upon differences between the Cluster & Machine objects exposed via the API
server, and the actual cluster configuration & machines forming the cluster,
this WKS controller will apply the necessary changes (e.g. adding or removing
machines) to bring the cluster in the desired state.`,
	PreRun: preRun,
	Run:    run,
}

func preRun(cmd *cobra.Command, args []string) {
	if options.verbose {
		log.SetLevel(log.DebugLevel)
	}
}

func run(cmd *cobra.Command, args []string) {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("failed to get the coordinates of the API server: %v", err)
	}
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("failed to create Kubernetes client set: %v", err)
	}
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		log.Fatalf("failed to create the cluster manager: %v", err)
	}

	log.Info("registering scheme for all resources")
	if err := baremetalv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatal(err)
	}
	if err := clusterv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatal(err)
	}

	log.Info("registering controllers to the cluster manager")
	clusterReconciler, err := wks.NewClusterReconciler(mgr.GetClient(), mgr.GetEventRecorderFor(wks.ProviderName+"-controller"))
	if err != nil {
		log.Fatal(err)
	}
	if err = clusterReconciler.SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		log.Fatal(err)
	}

	var ctlrNamespace string
	{
		// Create another client as we can't use the manager's one until it is started
		client, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
		if err != nil {
			log.Fatalf("failed to create client: %s", err)
		}
		ctlrNamespace, err = initializeControllerNamespace(client)
		if err != nil {
			log.Fatalf("failed to get controller namespace x: %s", err)
		}
	}

	machineController, err := wks.NewMachineController(wks.MachineControllerParams{
		EventRecorder:       mgr.GetEventRecorderFor(wks.ProviderName + "-controller"),
		Client:              mgr.GetClient(),
		ClientSet:           clientSet,
		ControllerNamespace: ctlrNamespace,
		Verbose:             options.verbose,
	})
	if err != nil {
		log.Fatalf("failed to create the machine actuator: %v", err)
	}
	if err = machineController.SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		log.Fatal(err)
	}

	log.Info("starting the cluster manager")
	log.Fatal(mgr.Start(signals.SetupSignalHandler()))
}
