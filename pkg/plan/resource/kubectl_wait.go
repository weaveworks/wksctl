package resource

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
)

// KubectlWait waits for an object to reach a required state
type KubectlWait struct {
	resource.Base

	// Namespace specifies the namespace in which to search for the object being waited on
	WaitNamespace string `structs:"namespace"`
	// WaitType specifies the object type to wait for
	WaitType string `structs:"typeWaitedFor"`
	// WaitSelector, if not empty, specifies which instances of the type to wait for
	WaitSelector string `structs:"itemsWaitedFor"`
	// WaitCondition specifies the condition to wait for
	WaitCondition string `structs:"waitFor"`
	// WaitTimeout, if specified, indicates how long to wait for the WaitCondition to become true before failing (default 30s)
	WaitTimeout string `structs:"waitTimeout"`
}

var _ plan.Resource = plan.RegisterResource(&KubectlWait{})

// State implements plan.Resource.
func (kw *KubectlWait) State() plan.State {
	return resource.ToState(kw)
}

// Apply performs a "kubectl wait" as specified in the receiver.
func (kw *KubectlWait) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	if err := kubectlWait(runner, kubectlWaitArgs{
		WaitNamespace: kw.WaitNamespace,
		WaitCondition: kw.WaitCondition,
		WaitType:      kw.WaitType,
		WaitSelector:  kw.WaitSelector,
		WaitTimeout:   kw.WaitTimeout,
	}); err != nil {
		return false, err
	}

	return true, nil
}

type kubectlWaitArgs struct {
	// Namespace specifies the namespace in which to search for the object being waited on
	WaitNamespace string
	// WaitType specifies the object type to wait for
	WaitType string
	// WaitSelector, if non-empty, specifies the specific entities to "kubectl wait" on
	WaitSelector string
	// WaitCondition, if non-empty, makes kubectlWait do "kubectl wait --for=<value>" on the applied resource.
	WaitCondition string
	// WaitTimeout, if specified, indicates how long to wait for the WaitCondition to become true before failing
	WaitTimeout string
}

func kubectlWait(r plan.Runner, args kubectlWaitArgs) error {
	// Assume the objects to wait for should/will exist. Don't start the timeout until they are present
	for {
		cmd := fmt.Sprintf("kubectl get %q %s%s", args.WaitType, waitOn(args), waitNamespace(args))
		output, err := r.RunCommand(resource.WithoutProxy(cmd), nil)
		if err != nil || strings.Contains(output, "No resources found") {
			time.Sleep(500 * time.Millisecond)
		} else {
			break
		}
	}
	cmd := fmt.Sprintf("kubectl wait %q --for=%q%s%s%s",
		args.WaitType, args.WaitCondition, waitOn(args), waitTimeout(args), waitNamespace(args))
	if _, err := r.RunCommand(resource.WithoutProxy(cmd), nil); err != nil {
		return errors.Wrap(err, "kubectl wait")
	}
	return nil
}

func waitOn(args kubectlWaitArgs) string {
	if args.WaitSelector != "" {
		return fmt.Sprintf(" --selector=%q", args.WaitSelector)
	}
	return ""
}

func waitTimeout(args kubectlWaitArgs) string {
	if args.WaitTimeout != "" {
		return fmt.Sprintf(" --timeout=%q", args.WaitTimeout)
	}
	return ""
}

func waitNamespace(args kubectlWaitArgs) string {
	if args.WaitNamespace != "" {
		return fmt.Sprintf(" --namespace=%q", args.WaitNamespace)
	}
	return ""
}
