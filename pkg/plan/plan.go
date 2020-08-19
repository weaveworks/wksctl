package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/fatih/structs"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
)

// Runner is something that can realise a step.
type Runner interface {
	// RunCommand runs a command in a shell. This means cmd can be more than one
	// single command, it can be a full bourne shell script.
	RunCommand(cmd string, stdin io.Reader) (stdouterr string, err error)
}

// Resource is an atomic step of the plan.
type Resource interface {
	// State returns the state that this step will realize when applied.
	State() State
	// QueryState returns the current state of this step. For instance, if the step
	// describes the installation of a package, QueryState will return if the
	// package is actually installed and its version.
	QueryState(runner Runner) (State, error)

	// Apply this step and indicate whether downstream resources should be re-applied
	Apply(runner Runner, diff Diff) (propagate bool, err error)
	// Undo this step.
	Undo(runner Runner, current State) error
}

type RunError struct {
	ExitCode int
}

func (e *RunError) Error() string {
	return fmt.Sprintf("command exited with %d", e.ExitCode)
}

// Plan is a succession of Steps to produce a desired outcome.
type Plan struct {
	id            string
	resources     map[string]Resource
	graph         *graph
	undoCondition func(Runner, State) bool
}

var (
	dummyPlan    Resource = RegisterResource(&Plan{})
	planTypeName          = extractResourceTypeName(dummyPlan)
)

// ParamString is a parameterizable string for passing output from one resource
// to another. The model is:
//
// A "Run" command (and possibly others?) can take an extra "Output" parameter which is the
// address of a string variable. The variable will get populated with the output of the command.
//
// A downstream resource can pass a "ParamString" along with any associated string variable addresses
// and the parameters will get filled in at runtime (after the upstream resource has run).
// "plan.ParamString(template, params...)" will create a ParamString whose "String()" method will return
// an instantiated string. If no parameters are necessary, "object.String(str)" can be used to make the intent
// more clear.
//
// Examples:
//
//	var k8sVersion string
//
//  b.AddResource(
//		"install:cni:get-k8s-version",
//		&resource.Run{
//			Script: object.String("kubectl version | base64 | tr -d '\n'"),
//			Output: &k8sVersion,
//		},
//		plan.DependOn("kubeadm:init"),
//  ).AddResource(
//		"install:cni",
//		&resource.KubectlApply{
//			ManifestURL: plan.ParamString("https://cloud.weave.works/k8s/net?k8s-version=%s", &k8sVersion),
//		},
//		plan.DependOn("install:cni:get-k8s-version"),
//  )
//
//  var homedir string
//
//  b.AddResource(
//		"kubeadm:get-homedir",
//		&Run{Script: object.String("echo -n $HOME"), Output: &homedir},
//  ).AddResource(
//		"kubeadm:config:kubectl-dir",
//		&Dir{Path: plan.ParamString("%s/.kube", &homedir)},
//		plan.DependOn("kubeadm:get-homedir"),
//  ).AddResource(
//		"kubeadm:config:copy",
//		&Run{Script: plan.ParamString("cp /etc/kubernetes/admin.conf %s/.kube/config", &homedir)},
//		plan.DependOn("kubeadm:run-init", "kubeadm:config:kubectl-dir"),
//  ).AddResource(
//		"kubeadm:config:set-ownership",
//		&Run{Script: plan.ParamString("chown $(id -u):$(id -g) %s/.kube/config", &homedir)},
//		plan.DependOn("kubeadm:config:copy"),
//  )

type paramTemplateString struct {
	template string
	params   []*string
}

func (p *paramTemplateString) String() string {
	strs := make([]interface{}, len(p.params))
	for i := range p.params {
		strs[i] = *p.params[i]
	}
	return fmt.Sprintf(p.template, strs...)
}

func ParamString(template string, params ...*string) fmt.Stringer {
	if len(params) == 0 {
		return object.String(template)
	}
	return &paramTemplateString{template, params}
}
