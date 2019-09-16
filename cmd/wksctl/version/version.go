package version

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/version"
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Display wksctl version",
	Run:   versionRun,
}

func versionRun(cmd *cobra.Command, args []string) {
	fmt.Println(version.Version)
}
