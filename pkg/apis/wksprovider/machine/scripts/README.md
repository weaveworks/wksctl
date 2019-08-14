# Scripts

## Introduction

This package is responsible for grouping scripts and other assets required to
setup Kubernetes on machines running various operating systems.

By default, these files aren't packaged inside our Go binaries, and therefore
not callable by it, if sources aren't kept alongside it.

We therefore create a "virtual file system" using:
    https://github.com/shurcooL/vfsgen
to artificially package these, and be able to just ship the binary.

## How does it work

Given:

- `assets_generate.go` (naming convention imposed by `vfsgen`)
- `doc.go`
- `scripts_dev.go`
- and the following `Makefile` configuration:

```
SCRIPTS=$(shell find pkg/apis/wksprovider/machine/scripts -name '*.sh' -print)
pkg/apis/wksprovider/machine/scripts/scripts_vfsdata.go: $(SCRIPTS)
	go generate ./pkg/apis/wksprovider/machine/scripts

ALL_ASSETS = ... pkg/apis/wksprovider/machine/scripts/scripts_vfsdata.go
```

running `make` will eventually call `vfsgen` to read all the scripts and copy
their content to `pkg/apis/wksprovider/machine/scripts/scripts_vfsdata.go`,
which can later be used by the binary, instead of calling the scripts directly.
