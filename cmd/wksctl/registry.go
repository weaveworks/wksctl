package main

import (
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/quay"
	"github.com/weaveworks/wksctl/pkg/registry"
	"github.com/weaveworks/wksctl/pkg/utilities"
	v "github.com/weaveworks/wksctl/pkg/utilities/version"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
)

var registrySyncCmd = &cobra.Command{
	Use:    "registry-sync-commands",
	Short:  "Synchronize container images to an internal registry",
	Long:   "Generate docker commands to STDOUT to pull, tag, and push the WKS container images to the provided destination organization and registry.",
	PreRun: globalPreRun,
	Run:    registrySyncRun,
}

var registrySyncOptions struct {
	destRegistry         string
	destOrganization     string
	machinesManifestPath string
	versionsRange        string
}

func init() {
	registrySyncCmd.PersistentFlags().StringVar(&registrySyncOptions.destRegistry, "dest-registry", "localhost:1337", "Destination registry that will be used to push images to")
	registrySyncCmd.PersistentFlags().StringVar(&registrySyncOptions.destOrganization, "dest-organization", "wks", "Destination organization that will be used to push images to")
	registrySyncCmd.PersistentFlags().StringVar(&registrySyncOptions.machinesManifestPath, "machines", "", "Location of machines manifest")
	registrySyncCmd.PersistentFlags().StringVar(&registrySyncOptions.versionsRange, "versions", "", "Range of Kubernetes semantic versions, e.g.: \""+kubernetes.DefaultVersionsRange+"\"")
	rootCmd.AddCommand(registrySyncCmd)
}

func registrySyncRun(cmd *cobra.Command, args []string) {
	imagesSet := make(map[registry.Image]struct{}) // to deduplicate images.

	// Get WKS' container images:
	wksImages, err := quay.ListImages("wks", kubernetesVersionsRange())
	if err != nil {
		log.Fatal(err)
	}
	for _, image := range wksImages {
		imagesSet[image] = struct{}{}
	}

	// Get addons' images:
	for _, addon := range ListAddons() {
		addonImages, err := addon.ListImages()
		if err != nil {
			log.WithField("error", err).WithField("addon", addon.Name).Fatal("Failed to get addon's images.")
		}
		for _, image := range addonImages {
			imagesSet[image] = struct{}{}
		}
	}

	// Convert set back into a slice:
	images := make([]registry.Image, 0, len(imagesSet))
	for image := range imagesSet {
		images = append(images, image)
	}
	sort.Sort(registry.ByCoordinate(images))

	// Generate all commands:
	commands := make([]string, 0, 3*len(images))
	for _, sourceImage := range images {
		// Make a copy of the source image and overrides the registry:
		destImage := sourceImage
		destImage.Registry = registrySyncOptions.destRegistry
		// set the organization for the image
		destImage.User = registrySyncOptions.destOrganization
		commands = append(commands, sourceImage.CommandsToRetagAs(destImage)...)
	}

	// Print all commands:
	for _, command := range commands {
		fmt.Println(command)
	}
}

func kubernetesVersionsRange() string {
	if registrySyncOptions.machinesManifestPath != "" {
		version, err := extractKubernetesVersionFromMachines(registrySyncOptions.machinesManifestPath)
		if err != nil {
			log.Fatalf("Failed to extract Kubernetes version from machines manifest: %s", err)
		}
		return fmt.Sprintf("=%s", version)
	}
	if registrySyncOptions.versionsRange != "" {
		return registrySyncOptions.versionsRange
	}
	return v.AnyRange
}

func extractKubernetesVersionFromMachines(machinesManifestPath string) (string, error) {
	errorsHandler := func(machines []*clusterv1.Machine, errors field.ErrorList) ([]*clusterv1.Machine, error) {
		if len(errors) > 0 {
			utilities.PrintErrors(errors)
			return nil, apierrors.InvalidMachineConfiguration("%s failed validation", machinesManifestPath)
		}
		return machines, nil
	}
	machines, err := machine.ParseAndDefaultAndValidate(machinesManifestPath, errorsHandler)
	if err != nil {
		return "", err
	}
	return machines[0].Spec.Versions.Kubelet, nil
}
