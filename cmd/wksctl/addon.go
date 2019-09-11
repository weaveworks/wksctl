package main

import (
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/addons"
)

var addonCmd = &cobra.Command{
	Use:     "addon",
	Aliases: []string{"addons"},
	Short:   "Manipulate addons",
	PreRun:  globalPreRun,
}

// ListAddons lists all addons
func ListAddons() []addons.Addon {
	var results []addons.Addon

	addons := addons.List()
	for _, addon := range addons {
		results = append(results, addon)
	}

	return results
}

func init() {
	rootCmd.AddCommand(addonCmd)
}
