package resource

import (
	"context"

	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// RunScript is a resource running the script available at the provided path.
// Run doesn't realise any state, Apply will always run the given script.
type RunScript struct {
	base

	Path string      `structs:"path"`
	Args interface{} `structs:"args"`
}

var _ plan.Resource = plan.RegisterResource(&RunScript{})

// State implements plan.Resource.
func (r *RunScript) State() plan.State {
	return toState(r)
}

// Apply implements plan.Resource.
func (r *RunScript) Apply(ctx context.Context, runner plan.Runner, diff plan.Diff) (bool, error) {
	_, err := scripts.Run(ctx, r.Path, r.Args, runner)
	if err != nil {
		return false, err
	}
	return true, nil
}
