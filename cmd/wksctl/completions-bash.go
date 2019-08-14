package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var bashCompletionFilename = "wksctl_bash_completion.sh"

var bashCompletionCmd = &cobra.Command{
	Use:   "bash-completions",
	Short: "Generate bash completion scripts",
	Long: `To generate completion files, run

wksctl bash-completions [-o <completions_file|directory>]

and follow instructions for your OS to configure/install the completion file.
`,
	Run: func(cmd *cobra.Command, args []string) {
		if Output != "" {
			outfile, err := CreateFile(Output, bashCompletionFilename)
			if err != nil {
				log.Fatal(err)
			}
			rootCmd.GenBashCompletion(outfile)
		} else {
			rootCmd.GenBashCompletion(os.Stdout)
		}
	}}

func init() {
	bashCompletionCmd.Flags().StringVarP(
		&Output, "output", "o", "",
		"completion filename or directory (default stdout)")
	rootCmd.AddCommand(bashCompletionCmd)
}
