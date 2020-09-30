package registrysynccommands

import (
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/addons"
	"github.com/weaveworks/wksctl/pkg/registry"
)

var Cmd = &cobra.Command{
	Use:   "registry-sync-commands",
	Short: "Synchronize container images to an internal registry",
	Long:  "Generate docker commands to STDOUT to pull, tag, and push the WKS container images to the provided destination organization and registry.",
	Run:   registrySyncRun,
}

var registrySyncOptions struct {
	destRegistry         string
	destOrganization     string
	machinesManifestPath string
	versionsRange        string
}

func init() {
	Cmd.Flags().StringVar(&registrySyncOptions.destRegistry, "dest-registry", "localhost:1337", "Destination registry that will be used to push images to")
	Cmd.Flags().StringVar(&registrySyncOptions.destOrganization, "dest-organization", "wks", "Destination organization that will be used to push images to")
	Cmd.Flags().StringVar(&registrySyncOptions.machinesManifestPath, "machines", "", "Location of machines manifest")
	Cmd.Flags().StringVar(&registrySyncOptions.versionsRange, "versions", "", "Range of Kubernetes semantic versions, e.g.: \""+kubernetes.DefaultVersionsRange+"\"")
}

func registrySyncRun(cmd *cobra.Command, args []string) {
	imagesSet := make(map[registry.Image]struct{}) // to deduplicate images.

	// Get addons' images:
	for _, addon := range addons.List() {
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
