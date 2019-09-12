package main

import (
	"github.com/spf13/cobra"
)

var addonCmd = &cobra.Command{
	Use:     "addon",
	Aliases: []string{"addons"},
	Short:   "Manipulate addons",
}

func init() {
	rootCmd.AddCommand(addonCmd)
}
