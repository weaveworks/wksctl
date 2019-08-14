package tarball

import (
	"fmt"
	"os/exec"

	"github.com/weaveworks/wksctl/pkg/utilities"
)

type Tarball struct {
	// Path is the filesystem path of the tarball file.
	Path string
	// Compression should be "" for plain .tar files, "z" for gzipped files, "j" for bzip2'ed files.
	Compression string
}

// Check returns error if `tar t` fails, and no error otherwise.
func (t *Tarball) Check() error {
	flags := fmt.Sprintf("-t%s", t.Compression)
	out, err := exec.Command("tar", flags, "-f", t.Path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar failed: %v; combined output:\n%s", err, utilities.Indent(string(out), "\t"))
	}
	return nil
}

func (t *Tarball) Unpack(destDir string) error {
	flags := fmt.Sprintf("-x%s", t.Compression)
	out, err := exec.Command("tar", flags, "-f", t.Path, "-C", destDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar failed: %v; combined output:\n%s", err, utilities.Indent(string(out), "\t"))
	}
	return nil
}
