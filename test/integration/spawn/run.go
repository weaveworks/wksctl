package run

// Options holds configuration options for the Executor object.
type Options struct {
	// Verbose controls if Executor should print command output to stdout and stderr.
	Verbose bool
}

// Run is the top level API objects. It can be used to create executor,
// responsible for running a series of commands.
type Run struct {
	options Options
}

// New creates a new run context that can be used to run successive commands.
func New(options Options) *Run {
	return &Run{
		options: options,
	}
}

// NewExecutor creates an executor for a specific test.
func (r *Run) NewExecutor() *Executor {
	return &Executor{
		run:             r,
		showOutput:      r.options.Verbose,
		showBreadcrumbs: r.options.Verbose,
	}
}
