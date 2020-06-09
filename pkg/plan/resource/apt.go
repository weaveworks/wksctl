package resource

import (
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"github.com/weaveworks/wksctl/pkg/plan"

	log "github.com/sirupsen/logrus"
)

type aptInstaller struct {
	Runner plan.Runner
	// Command to use. Defaults to "apt-get". This is useful for testing.
	Command string
}

const env = "LC_ALL=C DEBIAN_FRONTEND=noninteractive"

func (a *aptInstaller) UpdateCache() error {
	flags := "--yes --quiet"
	cmd := fmt.Sprintf("%s '%s' %s update", env, a.CommandMaybeDefault(), flags)
	if out, err := wrapRetry(a.Runner).RunCommand(cmd, nil); err != nil {
		return fmt.Errorf("command %q failed: %v; output: %s", cmd, err, out)
	}
	return nil
}

func (a *aptInstaller) Install(name, suffix string) error {
	flags := "--yes --quiet --verbose-versions --no-install-recommends --allow-downgrades"
	cmd := fmt.Sprintf("%s '%s' %s install '%s%s'", env, a.CommandMaybeDefault(), flags, name, suffix)
	if out, err := wrapRetry(a.Runner).RunCommand(cmd, nil); err != nil {
		return fmt.Errorf("command %q failed: %v; output: %s", cmd, err, out)
	}
	return nil
}

func (a *aptInstaller) Upgrade(name, suffix string) error {
	flags := "--yes --quiet --verbose-versions --no-install-recommends --only-upgrade"
	cmd := fmt.Sprintf("%s '%s' %s install '%s%s'", env, a.CommandMaybeDefault(), flags, name, suffix)
	if out, err := wrapRetry(a.Runner).RunCommand(cmd, nil); err != nil {
		return fmt.Errorf("command %q failed: %v; output: %s", cmd, err, out)
	}
	return nil
}

func (a *aptInstaller) Purge(name string) error {
	flags := "--yes --quiet --verbose-versions --auto-remove"
	cmd := fmt.Sprintf("%s '%s' %s purge '%s'", env, a.CommandMaybeDefault(), flags, name)
	out, err := wrapRetry(a.Runner).RunCommand(cmd, nil)

	if _, ok := err.(*plan.RunError); ok {
		if strings.Contains(out, "E: Unable to locate package "+name) {
			return nil // the package isn't known to the package manager, so it's not installed - success
		}

		return fmt.Errorf("command %q failed: %v; output: %s", cmd, err, out)
	}

	return nil
}

func (a *aptInstaller) CommandMaybeDefault() string {
	if a.Command == "" {
		return "apt-get"
	}
	return a.Command
}

type retryingRunner struct {
	Runner  plan.Runner
	Retries int
}

func wrapRetry(r plan.Runner) plan.Runner {
	return &retryingRunner{
		Runner:  r,
		Retries: 30,
	}
}

func (r *retryingRunner) RunCommand(cmd string, stdin io.Reader) (string, error) {
	return r.runOp(func() (string, error) { return r.Runner.RunCommand(cmd, stdin) })
}

func (r *retryingRunner) runOp(op func() (string, error)) (string, error) {
	var out string
	var err error

	for i := 0; i < r.Retries; i++ {
		out, err = op()
		if _, ok := err.(*plan.RunError); !ok {
			// Not the case that our command returned a non-zero status. Hence not retrying.
			return out, err
		}

		if !strings.Contains(out, "Resource temporarily unavailable") {
			// Not the kind of error we want to retry on. Hence not retrying.
			return out, err
		}

		sleep := time.Duration(rand.Intn(10000)) * time.Millisecond
		log.Debugf("Retry #%d: Sleeping for %v", i, sleep)
		time.Sleep(sleep)
	}

	return out, err
}
