package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/addons"
)

var addonBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build addon manifests",
	Args:  addonBuildArgs,
	Run:   addonBuildRun,
}

var addonBuildOptions struct {
	outputDirectory string
	params          []string
	imageRepository string
}

func init() {
	addonBuildCmd.PersistentFlags().StringVarP(&addonBuildOptions.outputDirectory, "output-directory", "o", "", "manifest output directory")
	addonBuildCmd.PersistentFlags().StringVarP(&addonBuildOptions.imageRepository, "image-repository", "r", "", "use this container repository for addon images")
	addonBuildCmd.PersistentFlags().StringArrayVarP(&addonBuildOptions.params, "params", "p", nil, "addon input parameters e.g. --params foo=bar --params baz=qux")
	addonCmd.AddCommand(addonBuildCmd)
}

func addonBuildArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("build requires an addon name")
	}
	return nil
}

func parseParam(input string) (name, value string, err error) {
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected key=value pair, got '%s'", input)
	}

	return parts[0], parts[1], nil
}

func makeParams(input []string) (map[string]string, error) {
	output := make(map[string]string)

	for _, desc := range input {
		name, value, err := parseParam(desc)
		if err != nil {
			return nil, err
		}
		output[name] = value
	}

	return output, nil
}

func addonBuildRun(cmd *cobra.Command, args []string) {
	opts := &addonBuildOptions

	addon, err := addons.Get(args[0])
	if err != nil {
		log.Fatal(err)
	}

	params, err := makeParams(opts.params)
	if err != nil {
		log.Fatal(err)
	}

	if opts.outputDirectory != "" {
		os.MkdirAll(opts.outputDirectory, 0770)
	}

	addonOptions := addons.BuildOptions{
		OutputDirectory: opts.outputDirectory,
		Params:          params,
		ImageRepository: opts.imageRepository,
		YAML:            true,
	}

	if err := addon.ValidateOptions(&addonOptions); err != nil {
		log.Fatalf("invalid options: %v\n", err)
	}

	manifests, err := addon.Build(addonOptions)
	if err != nil {
		log.Fatal(err)
	}

	for _, filename := range manifests {
		fmt.Printf("wrote %s\n", filename)
	}
}
