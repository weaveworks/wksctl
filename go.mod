module github.com/weaveworks/wksctl

go 1.13

require (
	github.com/bitnami-labs/sealed-secrets v0.12.1
	github.com/blang/semver v3.5.1+incompatible
	github.com/cavaliercoder/go-rpm v0.0.0-20200122174316-8cb9fd9c31a8
	github.com/chanwit/plandiff v1.0.0
	github.com/dlespiau/kube-test-harness v0.0.0-20190930170435-ec3f93e1a754
	github.com/fatih/structs v1.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.4.0 // indirect
	github.com/google/go-jsonnet v0.16.0
	github.com/googleapis/gnostic v0.4.1 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/oleiade/reflections v1.0.0 // indirect
	github.com/pelletier/go-toml v1.8.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1 // indirect
	github.com/prometheus/procfs v0.0.11 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20200627165143-92b8a710ab6c
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/thanhpk/randstr v1.0.4
	github.com/weaveworks/footloose v0.0.0-20200609124411-8f3df89ea188
	github.com/weaveworks/go-checkpoint v0.0.0-20170503165305-ebbb8b0518ab
	github.com/weaveworks/launcher v0.0.0-20180824102238-59a4fcc32c9c
	github.com/whilp/git-urls v0.0.0-20191001220047-6db9661140c0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e // indirect
	golang.org/x/sys v0.0.0-20200413165638-669c56c373c4 // indirect
	golang.org/x/tools v0.0.0-20200502202811-ed308ab3e770 // indirect
	gomodules.xyz/jsonpatch/v2 v2.1.0 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/oleiade/reflections.v1 v1.0.0
	gopkg.in/src-d/go-git.v4 v4.13.1
	k8s.io/api v0.18.5
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v0.18.5
	k8s.io/cluster-bootstrap v0.18.5
	k8s.io/kube-proxy v0.0.0
	k8s.io/kubernetes v1.18.5
	k8s.io/utils v0.0.0-20200619165400-6e3d28b6ed19
	sigs.k8s.io/cluster-api v0.3.6
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/appscode/jsonpatch => gomodules.xyz/jsonpatch/v2 v2.0.0+incompatible
	github.com/dlespiau/kube-test-harness => github.com/luxas/kube-test-harness v0.0.1
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.0
	k8s.io/api => k8s.io/api v0.18.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.5
	k8s.io/apiserver => k8s.io/apiserver v0.18.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.5
	k8s.io/client-go => k8s.io/client-go v0.18.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.5
	k8s.io/code-generator => k8s.io/code-generator v0.18.5
	k8s.io/component-base => k8s.io/component-base v0.18.5
	k8s.io/cri-api => k8s.io/cri-api v0.18.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.5
	k8s.io/kubectl => k8s.io/kubectl v0.18.5
	k8s.io/kubelet => k8s.io/kubelet v0.18.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.5
	k8s.io/metrics => k8s.io/metrics v0.18.5
	k8s.io/node-api => k8s.io/node-api v0.18.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.5
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.18.5
	k8s.io/sample-controller => k8s.io/sample-controller v0.18.5
)
