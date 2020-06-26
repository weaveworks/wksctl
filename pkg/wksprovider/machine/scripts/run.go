package scripts

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"text/template"

	log "github.com/sirupsen/logrus"
)

// runner is something that can run a command somewhere.
//
// N.B.: this interface is meant to match pkg/plan.Runner, and is used in order
// to decouple packages.
type runner interface {
	// RunCommand runs the provided command in a shell.
	// cmd can be more than one single command, it can be a full shell script.
	RunCommand(cmd string, stdin io.Reader) (stdouterr string, err error)
}

// Run applies the provided arguments to the provided script template,
// and executes the resulting script via the provided Runner.
//
// N.B.: this utility function is placed here so that 1) it is hopefully easier
// to known how to run the scripts provided in this package and 2) its
// implementation can be re-used by the various parts of WKS.
func Run(path string, args interface{}, runner runner) (string, error) {
	script, err := Scripts.Open(path)
	if err != nil {
		return "", err
	}
	defer script.Close()
	scriptContent, err := ioutil.ReadAll(script)
	if err != nil {
		return "", err
	}
	scriptTemplate, err := template.New(path).Parse(string(scriptContent))
	if err != nil {
		return "", err
	}
	var body bytes.Buffer
	err = scriptTemplate.Execute(&body, args)
	if err != nil {
		return "", err
	}
	stdOutErr, err := runner.RunCommand(body.String(), nil)
	if err != nil {
		log.WithFields(log.Fields{
			"stdOutErr": stdOutErr,
			"path":      path,
			"args":      args,
			"runner":    runner,
		}).Debug("failed to run script")
	}
	return stdOutErr, err
}

func WriteFile(content []byte, dstPath string, perm os.FileMode, runner runner) error {
	input := bytes.NewReader(content)
	cmd := fmt.Sprintf("mkdir -pv $(dirname %q) && sed -n 'w %s' && chmod 0%o %q", dstPath, dstPath, perm, dstPath)
	_, err := runner.RunCommand(cmd, input)
	return err
}
