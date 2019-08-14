package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	Output                string
	zshCompletionFileName = "wksctl_zsh_completions.sh"
)

var zshCompletionCmd = &cobra.Command{
	Use:   "zsh-completions",
	Short: "Generate zsh completion scripts",
	Long: `To load completion run

wksctl zsh-completions [-o <completions_file|directory>]

and follow instructions for your OS to configure/install the completion file.
`,
	Run: func(cmd *cobra.Command, args []string) {
		if Output != "" {
			outfile, err := CreateFile(Output, zshCompletionFileName)
			if err != nil {
				log.Fatal(err)
			}
			rootCmd.GenZshCompletion(outfile)
		} else {
			rootCmd.GenZshCompletion(os.Stdout)
		}
	}}

func init() {
	zshCompletionCmd.Flags().StringVarP(
		&Output, "output", "o", "",
		"completion filename or directory (default stdout)")
	rootCmd.AddCommand(zshCompletionCmd)
}
