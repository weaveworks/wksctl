package profile

import (
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/cmd/wksctl/profile/disable"
	"github.com/weaveworks/wksctl/cmd/wksctl/profile/enable"
)

var Cmd = &cobra.Command{
	Use:     "profile",
	Aliases: []string{"profile"},
	Short:   "Profile management",
}

func init() {
	Cmd.AddCommand(enable.Cmd)
	Cmd.AddCommand(disable.Cmd)
}
