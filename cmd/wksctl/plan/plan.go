package plan

import (
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/cmd/wksctl/plan/view"
)

// Cmd represents the plan command
var Cmd = &cobra.Command{
	Use:    "plan",
	Hidden: true,
	Short:  "Debugging commands for the cluster plan.",
}

func init() {
	Cmd.AddCommand(view.Cmd)
}
