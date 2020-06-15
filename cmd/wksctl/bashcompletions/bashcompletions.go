package bashcompletions

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/utilities"
)

var (
	output                 string
	bashCompletionFilename = "wksctl_bash_completion.sh"
)

var Cmd = &cobra.Command{
	Use:   "bash-completions",
	Short: "Generate bash completion scripts",
	Long: `To generate completion files, run

wksctl bash-completions [-o <completions_file|directory>]

and follow instructions for your OS to configure/install the completion file.
`,
	Run: func(cmd *cobra.Command, args []string) {
		if output != "" {
			outfile, err := utilities.CreateFile(output, bashCompletionFilename)
			if err != nil {
				log.Fatal(err)
			}
			if err = cmd.Root().GenBashCompletion(outfile); err != nil {
				log.Fatal(err)
			}
		} else {
			if err := cmd.Root().GenBashCompletion(os.Stdout); err != nil {
				log.Fatal(err)
			}
		}
	}}

func init() {
	Cmd.Flags().StringVarP(
		&output, "output", "o", "",
		"completion filename or directory (default stdout)")
}
