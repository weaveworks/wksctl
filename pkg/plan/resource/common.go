package resource

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/plan"
)

func removeFile(remotePath string, runner plan.Runner) error {
	if stdouterr, err := runner.RunCommand(fmt.Sprintf("rm -f %q", remotePath), nil); err != nil {
		log.WithField("stdouterr", stdouterr).WithField("path", remotePath).Debugf("failed to delete file")
		return errors.Wrapf(err, "failed to delete %q", remotePath)
	}
	return nil
}

type PkgType string

const (
	PkgTypeDeb  PkgType = "Deb"
	PkgTypeRPM  PkgType = "RPM"
	PkgTypeRHEL PkgType = "RHEL"
)
