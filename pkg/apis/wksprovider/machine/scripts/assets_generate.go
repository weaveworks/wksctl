// +build ignore

package main

import (
	"log"

	"github.com/shurcooL/vfsgen"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
)

func main() {
	err := vfsgen.Generate(scripts.Scripts, vfsgen.Options{
		PackageName:  "scripts",
		BuildTags:    "!dev",
		VariableName: "Scripts",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
