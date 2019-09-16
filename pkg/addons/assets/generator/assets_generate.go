package main

import (
	"log"

	"github.com/shurcooL/vfsgen"
	"github.com/weaveworks/wksctl/pkg/addons/assets"
)

func main() {
	err := vfsgen.Generate(assets.Assets, vfsgen.Options{
		PackageName:  "assets",
		BuildTags:    "!dev",
		VariableName: "Assets",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
