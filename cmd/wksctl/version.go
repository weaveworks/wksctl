package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display wksctl version",
	Run:   versionRun,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func versionRun(cmd *cobra.Command, args []string) {
	fmt.Println(version.Version)
}
