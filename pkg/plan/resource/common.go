package resource

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/plan"
)

func removeFile(ctx context.Context, remotePath string, runner plan.Runner) error {
	if stdouterr, err := runner.RunCommand(ctx, fmt.Sprintf("rm -f %q", remotePath), nil); err != nil {
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
