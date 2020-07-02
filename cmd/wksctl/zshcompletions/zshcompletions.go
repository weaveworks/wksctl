package zshcompletions

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/utilities"
)

var (
	output                string
	zshCompletionFileName = "wksctl_zsh_completions.sh"
)

var Cmd = &cobra.Command{
	Use:   "zsh-completions",
	Short: "Generate zsh completion scripts",
	Long: `To load completion run

wksctl zsh-completions [-o <completions_file|directory>]

and follow instructions for your OS to configure/install the completion file.
`,
	Run: func(cmd *cobra.Command, args []string) {
		if output != "" {
			outfile, err := utilities.CreateFile(output, zshCompletionFileName)
			if err != nil {
				log.Fatal(err)
			}
			if err = cmd.Root().GenZshCompletion(outfile); err != nil {
				log.Fatal(err)
			}
		} else {
			if err := cmd.Root().GenZshCompletion(os.Stdout); err != nil {
				log.Fatal(err)
			}
		}
	}}

func init() {
	Cmd.Flags().StringVarP(
		&output, "output", "o", "",
		"completion filename or directory (default stdout)")
}
