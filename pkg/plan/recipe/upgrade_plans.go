package recipe

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
)

type NodeType int

const (
	OriginalMaster NodeType = iota
	SecondaryMaster
	Worker
)

// BuildUpgradePlan creates a sub-plan to run upgrade using respective package management commands.
func BuildUpgradePlan(pkgType resource.PkgType, k8sVersion string, ntype NodeType) plan.Resource {
	b := plan.NewBuilder()

	switch pkgType {
	case resource.PkgTypeRPM, resource.PkgTypeRHEL:
		b.AddResource(
			"upgrade:node-unlock-kubernetes",
			&resource.Run{Script: object.String("yum versionlock delete 'kube*' || true")})
		b.AddResource(
			"upgrade:node-install-kubeadm",
			&resource.RPM{Name: "kubeadm", Version: k8sVersion, DisableExcludes: "kubernetes"},
			plan.DependOn("upgrade:node-unlock-kubernetes"))
	case resource.PkgTypeDeb:
		b.AddResource(
			"upgrade:node-unlock-kubernetes",
			&resource.Run{Script: object.String("apt-mark unhold 'kube*' || true")})
		b.AddResource(
			"upgrade:node-install-kubeadm",
			&resource.Deb{Name: "kubeadm", Suffix: "=" + k8sVersion + "-00"},
			plan.DependOn("upgrade:node-unlock-kubernetes"))
	}
	//
	// For secondary masters
	// version >= 1.16.0 uses: kubeadm upgrade node
	// version >= 1.14.0 && < 1.16.0 uses: kubeadm upgrade node experimental-control-plane
	//
	secondaryMasterUpgradeControlPlaneFlag := ""
	if lt, err := version.LessThan(k8sVersion, "v1.16.0"); err == nil && lt {
		secondaryMasterUpgradeControlPlaneFlag = "experimental-control-plane"
	}

	switch ntype {
	case OriginalMaster:
		b.AddResource(
			"upgrade:node-kubeadm-upgrade",
			&resource.Run{Script: object.String(fmt.Sprintf("kubeadm upgrade plan && kubeadm upgrade apply -y %s", k8sVersion))},
			plan.DependOn("upgrade:node-install-kubeadm"))
	case SecondaryMaster:
		b.AddResource(
			"upgrade:node-kubeadm-upgrade",
			&resource.Run{Script: object.String(fmt.Sprintf("kubeadm upgrade node %s", secondaryMasterUpgradeControlPlaneFlag))},
			plan.DependOn("upgrade:node-install-kubeadm"))
	case Worker:
		b.AddResource(
			"upgrade:node-kubeadm-upgrade",
			&resource.Run{Script: object.String(fmt.Sprintf("kubeadm upgrade node config --kubelet-version %s", k8sVersion))},
			plan.DependOn("upgrade:node-install-kubeadm"))
	}

	switch pkgType {
	case resource.PkgTypeRPM, resource.PkgTypeRHEL:
		b.AddResource(
			"upgrade:node-kubelet",
			&resource.RPM{Name: "kubelet", Version: k8sVersion, DisableExcludes: "kubernetes"},
			plan.DependOn("upgrade:node-kubeadm-upgrade"))
		b.AddResource(
			"upgrade:node-restart-kubelet",
			&resource.Run{Script: object.String("systemctl restart kubelet")},
			plan.DependOn("upgrade:node-kubelet"))
		b.AddResource(
			"upgrade:node-kubectl",
			&resource.RPM{Name: "kubelet", Version: k8sVersion, DisableExcludes: "kubernetes"},
			plan.DependOn("upgrade:node-restart-kubelet"))
		b.AddResource(
			"upgrade:node-lock-kubernetes",
			&resource.Run{Script: object.String("yum versionlock add 'kube*' || true")},
			plan.DependOn("upgrade:node-kubectl"))
	case resource.PkgTypeDeb:
		b.AddResource(
			"upgrade:node-kubelet",
			&resource.Deb{Name: "kubelet", Suffix: "=" + k8sVersion + "-00"},
			plan.DependOn("upgrade:node-kubeadm-upgrade"))
		b.AddResource(
			"upgrade:node-restart-kubelet",
			&resource.Run{Script: object.String("systemctl restart kubelet")},
			plan.DependOn("upgrade:node-kubelet"))
		b.AddResource(
			"upgrade:node-kubectl",
			&resource.Deb{Name: "kubectl", Suffix: "=" + k8sVersion + "-00"},
			plan.DependOn("upgrade:node-restart-kubelet"))
		b.AddResource(
			"upgrade:node-lock-kubernetes",
			&resource.Run{Script: object.String("apt-mark hold 'kube*' || true")},
			plan.DependOn("upgrade:node-kubectl"))
	}

	p, err := b.Plan()
	if err != nil {
		log.Fatalf("%v", err)
	}
	return &p
}
