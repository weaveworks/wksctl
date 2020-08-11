package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// KubectlAnnotateSingleNode is a resource to apply an annotation to the only node in a cluster
type KubectlAnnotateSingleNode struct {
	base

	Key   string // Which annotation to apply
	Value string // Value of annotation
}

var _ plan.Resource = plan.RegisterResource(&KubectlAnnotateSingleNode{})

// State implements plan.Resource.
func (ka *KubectlAnnotateSingleNode) State() plan.State {
	return toState(ka)
}

// Apply fetches the node name and performs a "kubectl annotate".
func (ka *KubectlAnnotateSingleNode) Apply(ctx context.Context, runner plan.Runner, diff plan.Diff) (bool, error) {
	output, err := runner.RunCommand(ctx, withoutProxy("kubectl get nodes -o name"), nil)
	if err != nil {
		return false, errors.Wrapf(err, "could not fetch node name to annotate")
	}

	nodeName := strings.Trim(output, " \n")
	if strings.Contains(nodeName, "\n") {
		return false, fmt.Errorf("unexpected output in node name: %q", output)
	}

	cmd := fmt.Sprintf("kubectl annotate %q %s=%q", nodeName, ka.Key, ka.Value)

	if stdouterr, err := runner.RunCommand(ctx, withoutProxy(cmd), nil); err != nil {
		return false, errors.Wrapf(err, "failed to apply annotation %s on %s; output %s", ka.Key, nodeName, stdouterr)
	}

	return true, nil
}
