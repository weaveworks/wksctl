package resource

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// XXX: Expose file permission (if needed?)

// File represents a file on the file system.
type File struct {
	// Source is a path to a local file. Only of of (Source, Content) can be
	// specified at once.
	Source string `structs:"source,omitempty"`
	// Content is the file content. Only of of (Source, Content) can be specified
	// at once.
	Content string `structs:"content,omitempty"`
	// Destination is the file destination path (required).
	Destination string `structs:"destination"`
	// File MD5 checksum. We use md5sum as it's part of coreutils and even part of
	// the default alpine image.
	Checksum string `structs:"checksum" plan:"hide"`
}

var _ plan.Resource = plan.RegisterResource(&File{})

// State implements plan.Resource.
func (f *File) State() plan.State {
	return toState(f)
}

// QueryState implements plan.Resource.
func (f *File) QueryState(runner plan.Runner) (plan.State, error) {
	output, err := runner.RunCommand(fmt.Sprintf("md5sum %s", f.Destination), nil)
	// XXX: this error message is actually locale dependent!
	if err != nil && strings.Contains(output, "No such file or directory") {
		return plan.EmptyState, nil
	}
	if err != nil {
		return plan.EmptyState, fmt.Errorf("Query file %s failed: %v -- %s", f.Destination, err, output)
	}
	fields := strings.Fields(line(output))
	state := f.State()
	state["checksum"] = fmt.Sprintf("md5:%s", fields[0])
	return state, nil
}

func checksumFromFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

func checksumFromString(content string) []byte {
	h := md5.New()
	_, _ = io.WriteString(h, content)
	return h.Sum(nil)
}

func (f *File) computeChecksum() error {
	if f.Checksum != "" {
		return nil
	}

	var sum []byte
	var err error
	if f.Source != "" {
		sum, err = checksumFromFile(f.Source)
	} else {
		sum = checksumFromString(f.Content)
	}
	if err != nil {
		return err
	}
	f.Checksum = fmt.Sprintf("md5:%x", sum)
	return nil
}

func (f *File) content() ([]byte, error) {
	if f.Source != "" {
		return ioutil.ReadFile(f.Source)
	}
	return []byte(f.Content), nil
}

// Apply implements plan.Resource.
func (f *File) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	if err := f.computeChecksum(); err != nil {
		return false, errors.Wrapf(err, "file: %s", f.Destination)
	}

	if f.State().Equal(diff.CurrentState) {
		return false, nil
	}

	content, err := f.content()
	if err != nil {
		return false, err
	}

	return true, scripts.WriteFile(content, f.Destination, 0660, runner)
}

// Undo implements plan.Resource.
func (f *File) Undo(runner plan.Runner, current plan.State) error {
	// Not checking checksum on Undo since File resources are being
	// used to undo actions taken within commands like kubeadminit.
	// In some cases we need to make sure files that would have been
	// created by the command are gone but we don't know if they've been
	// created or not.
	_, err := runner.RunCommand(fmt.Sprintf("rm -f %s", f.Destination), nil)
	return err
}
