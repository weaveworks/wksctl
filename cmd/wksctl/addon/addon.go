package addon

import (
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/cmd/wksctl/addon/build"
	"github.com/weaveworks/wksctl/cmd/wksctl/addon/list"
	"github.com/weaveworks/wksctl/cmd/wksctl/addon/show"
)

var Cmd = &cobra.Command{
	Use:     "addon",
	Aliases: []string{"addons"},
	Short:   "Manipulate addons",
}

func init() {
	Cmd.AddCommand(build.Cmd)
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(show.Cmd)
}
