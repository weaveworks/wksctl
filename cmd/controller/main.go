package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/apis"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/wksctl"
	machineutil "github.com/weaveworks/wksctl/pkg/cluster/machine"
	"k8s.io/client-go/kubernetes"
	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	capicluster "sigs.k8s.io/cluster-api/pkg/controller/cluster"
	capimachine "sigs.k8s.io/cluster-api/pkg/controller/machine"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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

func initializeControllerNamespace() (string, error) {
	controllerNamespace, err := machineutil.GetKubernetesNamespaceFromMachines()
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
	ctlrNamespace, err := initializeControllerNamespace()
	if err != nil {
		log.Fatalf("failed to get controller namespace: %s", err)
	}
	log.Info("initializing machine actuator")
	machineActuator, err := wks.NewMachineActuator(wks.MachineActuatorParams{
		EventRecorder:       mgr.GetRecorder(wks.ProviderName + "-controller"),
		Client:              mgr.GetClient(),
		ClientSet:           clientSet,
		ControllerNamespace: ctlrNamespace,
		Scheme:              mgr.GetScheme(),
		Verbose:             options.verbose,
	})
	if err != nil {
		log.Fatalf("failed to create the machine actuator: %v", err)
	}

	log.Info("initializing cluster actuator")
	clusterActuator, err := wks.NewClusterActuator(wks.ClusterActuatorParams{
		EventRecorder: mgr.GetRecorder(wks.ProviderName + "-controller"),
		Client:        mgr.GetClient(),
		ClientSet:     clientSet,
		Scheme:        mgr.GetScheme(),
	})
	if err != nil {
		log.Fatalf("failed to create the cluster actuator: %v", err)
	}

	clustercommon.RegisterClusterProvisioner(wks.ProviderName, clusterActuator)

	log.Info("registering scheme for all resources")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatal(err)
	}
	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatal(err)
	}

	log.Info("registering controllers to the cluster manager")
	capimachine.AddWithActuator(mgr, machineActuator)
	capicluster.AddWithActuator(mgr, clusterActuator)

	log.Info("starting the cluster manager")
	log.Fatal(mgr.Start(signals.SetupSignalHandler()))
}
