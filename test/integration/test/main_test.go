package test

import (
	"flag"
	"fmt"
	"log"
	"os"
	"testing"

	harness "github.com/dlespiau/kube-test-harness"
	"github.com/dlespiau/kube-test-harness/logger"
	spawn "github.com/weaveworks/wksctl/test/integration/spawn"
)

var (
	// cmd is the name of the CLI binary to test.
	cmd string

	// kubectl is the path to a recent kubectl command
	kubectl string
)

var options struct {
	run struct {
		interactive bool
	}
	terraform struct {
		outputPath string
	}
	Tags struct {
		WKSK8sKrb5Server   string
		WKSMockAuthzServer string
	}
	Secrets struct {
		QuaySecret string
	}
}

var (
	run  *spawn.Run
	kube *harness.Harness
)

const kubectlURL = "https://storage.googleapis.com/kubernetes-release/release/v%s/bin/linux/amd64/kubectl"

func downloadKubectl(version string) error {
	const file = "kubectl"

	url := fmt.Sprintf(kubectlURL, version)
	if err := downloadFileWithRetries(file, url, 3); err != nil {
		return err
	}

	stats, err := os.Stat(file)
	if err != nil {
		return err
	}
	mode := stats.Mode()
	mode |= 0111
	return os.Chmod(file, mode)
}

func TestMain(m *testing.M) {
	flag.StringVar(&cmd, "cmd", "wksctl", "path of the command under test")
	flag.BoolVar(&options.run.interactive, "run.interactive", false, "print command output when running them")
	flag.StringVar(&options.terraform.outputPath, "terraform.output", "/tmp/terraform_output.json", "JSON file holding terraform output")
	flag.StringVar(&options.Tags.WKSK8sKrb5Server, "tags.wks-k8s-krb5-server", "latest", "Tag of wks-k8s-krb5-server image to use in test")
	flag.StringVar(&options.Tags.WKSMockAuthzServer, "tags.wks-mock-authz-server", "latest", "Tag of wks-mock-authz-server image to use in test")
	flag.Parse()

	options.Secrets.QuaySecret = os.Getenv("QUAY_SECRET")

	// Setup the executable runner.
	run = spawn.New(spawn.Options{
		Verbose: options.run.interactive,
	})

	// Setup the Kubernetes testing package.
	kubeOptions := harness.Options{
		LogLevel: logger.Debug,
	}
	if options.run.interactive {
		kubeOptions.Logger = &logger.PrintfLogger{}
	}
	kube = harness.New(kubeOptions)

	// Download kubectl!
	if err := downloadKubectl("1.10.5"); err != nil {
		log.Fatalf("could not download kubectl: %v", err)
	}
	kubectl = "./kubectl"

	os.Exit(kube.Run(m))
}
