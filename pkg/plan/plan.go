package plan

import (
	"fmt"

	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
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
