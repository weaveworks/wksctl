package resource

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/weaveworks/wksctl/pkg/plan"
)

// dpkgQuerier can ask dpkg about packages installed in the system.
type dpkgQuerier struct {
	Runner plan.Runner
	// Command to use. Defaults to "dpkg-query". This is useful for testing.
	Command string
}

// debPkgInfo identifies a .deb package.
type debPkgInfo struct {
	Name, Version string
}

func (d *dpkgQuerier) ShowInstalled(ctx context.Context, name string) ([]debPkgInfo, error) {
	// Run dpkg-query.
	const sep = "\t"
	formatFields := []string{"${Package}", "${Version}"}

	cmd := fmt.Sprintf("'%s' --showformat '%s' -W '%s'",
		d.CommandMaybeDefault(), strings.Join(formatFields, sep)+"\n", name)
	out, err := d.Runner.RunCommand(ctx, cmd, nil)

	// Handle "package not found".
	if err != nil {
		const ExitCodePkgNotFound = 1
		if err, ok := err.(*plan.RunError); ok && err.ExitCode == ExitCodePkgNotFound {
			return nil, nil
		}
	}

	// Handle runtime errors.
	if err != nil {
		return nil, fmt.Errorf("dpkg: command %q failed: %v", cmd, err)
	}

	// Parse the package list.
	var pkgs []debPkgInfo
	lines := bufio.NewScanner(strings.NewReader(out))
	for lines.Scan() {
		line := lines.Text()
		fields := strings.SplitN(line, sep, len(formatFields)+1)
		if len(fields) != len(formatFields) {
			return nil, fmt.Errorf("cannot parse output line (bad number of fields): %v", line)
		}
		pkgs = append(pkgs, debPkgInfo{
			Name:    fields[0],
			Version: fields[1],
		})
	}
	if err := lines.Err(); err != nil {
		return nil, err
	}

	return pkgs, nil
}

func (d *dpkgQuerier) CommandMaybeDefault() string {
	if d.Command == "" {
		return "dpkg-query"
	}
	return d.Command
}
