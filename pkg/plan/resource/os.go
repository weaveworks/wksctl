package resource

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/plan"
	reflections "gopkg.in/oleiade/reflections.v1"
)

// OS is a set of OS properties.
type OS struct {

	// Name is the OS name, eg. 'centos' or 'debian'. On systemd OSes, this is the ID
	// field of /etc/os-release. See:
	//   https://www.freedesktop.org/software/systemd/man/os-release.html
	Name string `structs:"Name"`

	// Version the OS version. On systemd OSes, this is the VERSION_ID field of
	// /etc/os-release. See:
	//   https://www.freedesktop.org/software/systemd/man/os-release.html
	Version    string `structs:"Version"`
	MachineID  string `structs:"MachineID"`
	SystemUUID string `structs:"SystemUUID"`

	runner        plan.Runner
	factsGathered bool
}

func NewOS(r plan.Runner) (*OS, error) {
	osr := &OS{runner: r}
	_, err := osr.Apply(r, plan.EmptyDiff())
	if err != nil {
		return nil, err
	}
	return osr, nil
}

type ReadFileCmdFunc func(s ...string) string

type GatherFactFunc func(o *OS, r plan.Runner) error

type factGatheringParams struct {
	paramName   string `structs:"pname"`
	readFileCmd string `structs:"rfcmd"`
	cmdErr      string `structs:"iderr"`
	blankErr    string `structs:"idblankerr,omitempty"`
}

func newFactGatheringParams(pname string, fnames ...string) factGatheringParams {
	return factGatheringParams{
		paramName:   pname,
		readFileCmd: readFileCommand(fnames...),
		cmdErr:      fmt.Sprintf("Could not get %s", pname),
		blankErr:    fmt.Sprintf("%s is blank", pname),
	}
}

func readFileCommand(fnames ...string) string {
	fileCmds := make([]string, len(fnames))
	for i, fname := range fnames {
		fileCmds[i] = fmt.Sprintf("cat %s", fname)
	}
	// We need to disable stderr output here, otherwise the output will be clobbered with such output
	// if the first file is not existent
	return strings.Join(fileCmds, " 2>/dev/null || ") + " 2>/dev/null"
}

var (
	machineIDParams               = newFactGatheringParams("MachineID", "/etc/machine-id", "/var/lib/dbus/machine-id")
	sysUuidParams                 = newFactGatheringParams("SystemUUID", "/sys/class/dmi/id/product_uuid", "/etc/machine-id")
	_               plan.Resource = plan.RegisterResource(&OS{})
)

// State implements plan.Resource.
func (p *OS) State() plan.State {
	return toState(p)
}

var gatherFuncs []GatherFactFunc = []GatherFactFunc{
	getOSRelease,
	getMachineID,
	getSystemUUID,
}

func (p *OS) gatherFacts(r plan.Runner) error {
	for _, f := range gatherFuncs {
		err := f(p, r)
		if err != nil {
			log.Errorf("error: %s\n", err.Error())
			return err
		}
	}
	return nil
}

func (p *OS) query(r plan.Runner) error {
	err := p.gatherFacts(r)
	if err != nil {
		return err
	}
	return nil
}

// QueryState implements plan.Resource.
func (p *OS) QueryState(r plan.Runner) (plan.State, error) {
	err := p.query(r)
	if err != nil {
		return plan.EmptyState, err
	}
	return p.State(), nil
}

// Apply implements plan.Resource.
func (p *OS) Apply(r plan.Runner, _ plan.Diff) (bool, error) {
	err := p.query(r)
	if err != nil {
		return false, err
	}
	return false, nil
}

func (p *OS) Undo(r plan.Runner, current plan.State) error {
	return nil
}

func getOSRelease(p *OS, r plan.Runner) error {
	output, err := r.RunCommand("cat /etc/os-release", nil)
	if err != nil {
		return err
	}
	name := keyval(output, "ID")
	if name == "" {
		return fmt.Errorf("os: getOSRelease: could not query ID from output %s\n", output)
	}
	err = reflections.SetField(p, "Name", name)
	if err != nil {
		return err
	}
	version := keyval(output, "VERSION_ID")
	if version == "" {
		return errors.New("os: getOSRelease: could not query VERSION_ID")
	}
	return reflections.SetField(p, "Version", version)
}

func (p *OS) HasCommand(cmd string) (bool, error) {
	// http://stackoverflow.com/questions/592620/how-to-check-if-a-program-exists-from-a-bash-script
	_, err := p.runner.RunCommand(fmt.Sprintf("command -v -- %q >/dev/null 2>&1", cmd), nil)
	if err == nil {
		// Command found.
		return true, nil
	}

	if _, ok := err.(*plan.RunError); ok {
		// Non-zero exit code. It means: Command not found.
		return false, nil
	}

	// Runtime error.
	return false, err
}

func (p *OS) HasSELinuxEnabled() (bool, error) {
	const cmd = "selinuxenabled"

	if hasCmd, err := p.HasCommand(cmd); err != nil {
		// Inconclusive.
		return false, err
	} else if !hasCmd {
		// No SELinux tools installed.
		return false, nil
	}

	if _, err := p.runner.RunCommand(cmd, nil); err == nil {
		// SELinux not disabled (that is, enforcing or permissive).
		return true, nil
	} else if err, ok := err.(*plan.RunError); ok && err.ExitCode == 1 {
		// SELinux disabled.
		return false, nil
	} else {
		// Inconclusive.
		return false, err
	}
}

func (p *OS) IsSELinuxPermissive() (bool, error) {
	if _, err := p.runner.RunCommand("sestatus | grep 'Current mode' | grep permissive", nil); err == nil {
		return true, nil
	} else if err, ok := err.(*plan.RunError); ok && err.ExitCode == 1 {
		// not conform with selinux permissive, may be not permissive, maybe command not found
		return false, nil
	} else {
		return false, err
	}
}

func (p *OS) IsOSInContainerVM() (bool, error) {
	output, err := p.runner.RunCommand("cat /proc/1/environ", nil)
	return strings.Contains(output, "container=docker"), err
}

func (p *OS) GetMachineID(r plan.Runner) (string, error) {
	err := getMachineID(p, r)
	if err != nil {
		return "", err
	}
	id, err := p.State().GetString("MachineID")
	if err != nil {
		return "", err
	}
	return id, nil
}

func getMachineID(p *OS, r plan.Runner) error {
	return p.getValueFromFileContents(machineIDParams, r)
}

func (p *OS) GetSystemUUID(r plan.Runner) (string, error) {
	err := getSystemUUID(p, r)
	if err != nil {
		return "", err
	}
	id, err := p.State().GetString("SystemUUID")
	if err != nil {
		return "", err
	}
	return id, nil
}

func getSystemUUID(p *OS, r plan.Runner) error {
	return p.getValueFromFileContents(sysUuidParams, r)
}

func (p *OS) getValueFromFileContents(fgparams factGatheringParams, r plan.Runner) error {
	cmd := fgparams.readFileCmd
	output, err := r.RunCommand(cmd, nil)
	if err != nil {
		return errors.New(fgparams.cmdErr)
	}
	param := strings.TrimSpace(output)
	if len(param) == 0 {
		return errors.New(fgparams.blankErr)
	}
	return reflections.SetField(p, fgparams.paramName, param)
}
