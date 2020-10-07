package recipe

import (
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/manifests"
)

// BuildConfigMapPlan creates a plan to handle config maps
func BuildConfigMapPlan(manifests map[string][]byte, namespace string) plan.Resource {
	b := plan.NewBuilder()
	for name, manifest := range manifests {
		remoteName := fmt.Sprintf("config-map-%s", name)
		b.AddResource("install:"+remoteName, &resource.KubectlApply{Filename: object.String(remoteName), Manifest: manifest, Namespace: object.String(namespace)})
	}
	p, err := b.Plan()
	if err != nil {
		log.Fatalf("%v", err)
	}
	return &p
}

// BuildAddonPlan creates a plan containing all the addons from the cluster manifest
func BuildAddonPlan(clusterManifestPath string, addons map[string][][]byte) plan.Resource {
	b := plan.NewBuilder()
	for name, manifests := range addons {
		var previous *string
		for i, m := range manifests {
			resFile := fmt.Sprintf("%s-%02d", name, i)
			resName := "install:addon:" + resFile
			manRsc := &resource.KubectlApply{Manifest: m, Filename: object.String(resFile + ".yaml"), Namespace: object.String("addons")}

			if previous != nil {
				b.AddResource(resName, manRsc, plan.DependOn(*previous))
			} else {
				b.AddResource(resName, manRsc)
			}
			previous = &resName
		}
	}
	p, err := b.Plan()
	if err != nil {
		log.Fatalf("%v", err)
	}
	return &p
}
