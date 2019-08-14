package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wksctl",
	Short: "Weave Enterprise Kubernetes Subscription CLI",
}

var options struct {
	verbose bool
}

func globalPreRun(cmd *cobra.Command, args []string) {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	if options.verbose {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	rootCmd.PersistentFlags().BoolVarP(&options.verbose, "verbose", "v", false, "Enable verbose output")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
