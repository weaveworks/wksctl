package main

import (
	"context"
	"os"

	ot "github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/go-checkpoint"

	"github.com/weaveworks/wksctl/cmd/wksctl/addon"
	"github.com/weaveworks/wksctl/cmd/wksctl/apply"
	"github.com/weaveworks/wksctl/cmd/wksctl/applyaddons"
	"github.com/weaveworks/wksctl/cmd/wksctl/bashcompletions"
	initpkg "github.com/weaveworks/wksctl/cmd/wksctl/init"
	"github.com/weaveworks/wksctl/cmd/wksctl/kubeconfig"
	"github.com/weaveworks/wksctl/cmd/wksctl/plan"
	"github.com/weaveworks/wksctl/cmd/wksctl/profile"
	"github.com/weaveworks/wksctl/cmd/wksctl/registrysynccommands"
	"github.com/weaveworks/wksctl/cmd/wksctl/version"
	"github.com/weaveworks/wksctl/cmd/wksctl/zshcompletions"
	v "github.com/weaveworks/wksctl/pkg/version"
	"github.com/weaveworks/wksctl/utilities/tracing"
)

var rootCmd = &cobra.Command{
	Use:   "wksctl",
	Short: "Weave Kubernetes System CLI",

	PersistentPreRun: configureLogger,
}

var options struct {
	verbose bool
}

func configureLogger(cmd *cobra.Command, args []string) {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	if options.verbose {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	tracingCloser, err := tracing.SetupJaeger("wksctl")
	if err != nil {
		log.Fatalf("failed to set up Jaeger: %v", err)
	}
	defer tracingCloser.Close()

	sp := ot.StartSpan("wksctl")
	defer sp.Finish()
	ctx := ot.ContextWithSpan(context.Background(), sp)

	rootCmd.PersistentFlags().BoolVarP(&options.verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(addon.Cmd)
	rootCmd.AddCommand(apply.Cmd)
	rootCmd.AddCommand(applyaddons.Cmd)
	rootCmd.AddCommand(initpkg.Cmd)
	rootCmd.AddCommand(kubeconfig.Cmd)
	rootCmd.AddCommand(plan.Cmd)
	rootCmd.AddCommand(profile.Cmd)
	rootCmd.AddCommand(registrysynccommands.Cmd)
	rootCmd.AddCommand(version.Cmd)

	rootCmd.AddCommand(bashcompletions.Cmd)
	rootCmd.AddCommand(zshcompletions.Cmd)

	if checkResponse, err := checkpoint.Check(&checkpoint.CheckParams{
		Product: "wksctl",
		Version: v.Version,
	}); err == nil && checkResponse.Outdated {
		log.Infof("wksctl version %s is available; please update at %s",
			checkResponse.CurrentVersion, checkResponse.CurrentDownloadURL)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		sp.Finish()
		os.Exit(1)
	}

}
