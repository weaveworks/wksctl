// +build ignore

package main

import (
	"log"

	"github.com/shurcooL/vfsgen"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/crds"
)

func main() {
	err := vfsgen.Generate(crds.CRDs, vfsgen.Options{
		PackageName:  "crds",
		BuildTags:    "!dev",
		VariableName: "CRDs",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
