package show

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/addons"
)

var Cmd = &cobra.Command{
	Use:   "show",
	Short: "Show details about an addon",
	Args:  addonShowArgs,
	Run:   addonShowRun,
}

func addonShowArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("show requires an addon name")
	}
	return nil
}

func addonShowRun(cmd *cobra.Command, args []string) {
	addon, err := addons.Get(args[0])
	if err != nil {
		log.Fatal(err)
	}

	const tabWidth = 4
	w := tabwriter.NewWriter(os.Stdout, 0, 0, tabWidth, ' ', 0)

	fmt.Fprintf(w, "Name\t%s\n", addon.Name)
	fmt.Fprintf(w, "Category\t%s\n", addon.Category)
	fmt.Fprintf(w, "Description\t%s\n", addon.Description)
	fmt.Fprintf(w, "Params\n")
	for _, param := range addon.Params {
		var required string
		if param.Required {
			required = "required, "
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t(%sdefault: '%s')\n", "", param.Name, param.Description, required, param.DefaultValue)
	}

	w.Flush()
}
