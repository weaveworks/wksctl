module github.com/weaveworks/wksctl

go 1.14

require (
	github.com/bitnami-labs/sealed-secrets v0.12.5
	github.com/blang/semver v3.5.1+incompatible
	github.com/dlespiau/kube-test-harness v0.0.0-20200706152414-7c811932d687
	github.com/ghodss/yaml v1.0.0
	github.com/google/go-jsonnet v0.16.0
	github.com/pkg/errors v0.9.1
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/thanhpk/randstr v1.0.4
	github.com/weaveworks/cluster-api-provider-existinginfra v0.2.2
	github.com/weaveworks/footloose v0.0.0-20200918140536-ff126705213e
	github.com/weaveworks/go-checkpoint v0.0.0-20170503165305-ebbb8b0518ab
	github.com/weaveworks/launcher v0.0.0-20180824102238-59a4fcc32c9c
	github.com/weaveworks/libgitops v0.0.2
	github.com/whilp/git-urls v0.0.0-20191001220047-6db9661140c0
	golang.org/x/crypto v0.0.0-20201016220609-9e8e0b390897
	golang.org/x/tools v0.0.0-20200708003708-134513de8882 // indirect
	gomodules.xyz/jsonpatch/v2 v2.1.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.3
	k8s.io/kubernetes v1.20.2
	sigs.k8s.io/cluster-api v0.3.6
	sigs.k8s.io/kustomize/kyaml v0.6.0 // indirect
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/appscode/jsonpatch => gomodules.xyz/jsonpatch/v2 v2.0.0+incompatible
	github.com/docker/docker => github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.0
	github.com/moby/spdystream => github.com/docker/spdystream v0.2.0
	k8s.io/api => k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.2
	k8s.io/apiserver => k8s.io/apiserver v0.20.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.2
	k8s.io/client-go => k8s.io/client-go v0.20.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.2
	k8s.io/code-generator => k8s.io/code-generator v0.20.2
	k8s.io/component-base => k8s.io/component-base v0.20.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.2
	k8s.io/cri-api => k8s.io/cri-api v0.20.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.2
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20210216185858-15cd8face8d6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.2
	k8s.io/kubectl => k8s.io/kubectl v0.20.2
	k8s.io/kubelet => k8s.io/kubelet v0.20.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.2
	k8s.io/metrics => k8s.io/metrics v0.20.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.2
	k8s.io/node-api => k8s.io/node-api v0.20.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.2
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.20.2
	k8s.io/sample-controller => k8s.io/sample-controller v0.20.2
)
