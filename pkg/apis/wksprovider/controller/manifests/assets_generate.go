// +build ignore

package main

import (
	"log"

	"github.com/shurcooL/vfsgen"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/manifests"
)

func main() {
	err := vfsgen.Generate(manifests.Manifests, vfsgen.Options{
		PackageName:  "manifests",
		BuildTags:    "!dev",
		VariableName: "Manifests",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
