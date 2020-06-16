.PHONY: all install clean generated images lint unit-tests check
.DEFAULT_GOAL := all

# Boilerplate for building Docker containers.
# All this must go at top of file I'm afraid.
IMAGE_PREFIX := docker.io/weaveworks/wksctl-
IMAGE_TAG := $(shell tools/image-tag)
GIT_REVISION := $(shell git rev-parse HEAD)
VERSION=$(shell git describe --always)
UPTODATE := .uptodate

# Every directory with a Dockerfile in it builds an image called
# $(IMAGE_PREFIX)<dirname>. Dependencies (i.e. things that go in the image)
# still need to be explicitly declared.
%/$(UPTODATE): %/Dockerfile %/*
	mkdir -p bin # Restrict the build context to bin, create it here if it doesn't exist.
	$(SUDO) docker build --build-arg=revision="$(GIT_REVISION)" -t "$(IMAGE_PREFIX)$(shell basename $(@D))" -f - bin < "$(@D)/Dockerfile"
	$(SUDO) docker tag "$(IMAGE_PREFIX)$(shell basename $(@D))" "$(IMAGE_PREFIX)$(shell basename $(@D)):$(IMAGE_TAG)"
	touch "$@"

# Get a list of directories containing Dockerfiles
DOCKERFILES := $(shell find . \
 -name docs         -prune -o \
 -name tools        -prune -o \
 -name rpm          -prune -o \
 -name build        -prune -o \
 -name environments -prune -o \
 -name test         -prune -o \
 -name examples     -prune -o \
 -type f -name 'Dockerfile' \
 -print \
)

UPTODATE_FILES := $(patsubst %/Dockerfile,%/$(UPTODATE),$(DOCKERFILES))
DOCKER_IMAGE_DIRS := $(patsubst %/Dockerfile,%,$(DOCKERFILES))
IMAGE_NAMES := $(foreach dir,$(DOCKER_IMAGE_DIRS),$(patsubst %,$(IMAGE_PREFIX)%,$(shell basename $(dir))))
images:
	$(info $(IMAGE_NAMES))
	@echo > /dev/null

# Define imagetag-golang, etc, for each image, which parses the dockerfile and
# prints an image tag. For example:
#     FROM golang:1.8.1-stretch
# in the "foo/Dockerfile" becomes:
#     $ make imagetag-foo
#     1.8.1-stretch
define imagetag_dep
.PHONY: imagetag-$(1)
$(patsubst $(IMAGE_PREFIX)%,imagetag-%,$(1)): $(patsubst $(IMAGE_PREFIX)%,%,$(1))/Dockerfile
	@cat $$< | grep "^FROM " | head -n1 | sed 's/FROM \(.*\):\(.*\)/\2/'
endef
$(foreach image, $(IMAGE_NAMES), $(eval $(call imagetag_dep, $(image))))

all: $(UPTODATE_FILES) binaries

check: all lint unit-tests container-tests

BINARIES = \
	bin/wksctl \
	bin/mock-authz-server \
	bin/mock-https-authz-server \
	bin/controller \
	$(NULL)

binaries: $(BINARIES)

godeps=$(shell go list -f '{{join .Deps "\n"}}' $1 | \
	   xargs go list -f \
	   '{{if not .Standard}}{{ $$dep := . }}{{range .GoFiles}}{{$$dep.Dir}}/{{.}} {{end}}{{end}}')

DEPS=$(call godeps,./cmd/wksctl)

ADDONS_SOURCES=$(shell find addons/ -print)
pkg/addons/assets/assets_vfsdata.go: $(ADDONS_SOURCES)
	go generate ./pkg/addons/assets

SCRIPTS=$(shell find pkg/apis/wksprovider/machine/scripts/all -name '*.sh' -print)
pkg/apis/wksprovider/machine/scripts/scripts_vfsdata.go: $(SCRIPTS)
	go generate ./pkg/apis/wksprovider/machine/scripts

MANIFESTS=$(shell find pkg/apis/wksprovider/controller/manifests/yaml -name '*.yaml' -print)
pkg/apis/wksprovider/controller/manifests/manifests_vfsdata.go: $(MANIFESTS)
	go generate ./pkg/apis/wksprovider/controller/manifests

CRDS=$(shell find pkg/apis/cluster-api/config/crds -name '*.yaml' -print)
pkg/apis/wksprovider/machine/os/crds_vfsdata.go: $(CRDS)
	go generate ./pkg/apis/wksprovider/machine/crds

generated: pkg/addons/assets/assets_vfsdata.go pkg/apis/wksprovider/controller/manifests/manifests_vfsdata.go pkg/apis/wksprovider/machine/scripts/scripts_vfsdata.go pkg/apis/wksprovider/machine/os/crds_vfsdata.go

bin/wksctl: $(DEPS) generated
bin/wksctl: cmd/wksctl/*.go
	CGO_ENABLED=0 GOARCH=amd64 go build -ldflags \
		"-X github.com/weaveworks/wksctl/pkg/version.Version=$(VERSION) -X github.com/weaveworks/wksctl/pkg/version.ImageTag=$(IMAGE_TAG)" \
		-o $@ cmd/wksctl/*.go

docker/controller/.uptodate: bin/controller docker/controller/Dockerfile
bin/controller: $(DEPS) generated
bin/controller: cmd/controller/*.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $@ cmd/controller/*.go

docker/mock-authz-server/.uptodate: bin/mock-authz-server docker/mock-authz-server/Dockerfile
bin/mock-authz-server: cmd/mock-authz-server/*.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $@ cmd/mock-authz-server/*.go

docker/mock-https-authz-server/.uptodate: bin/mock-https-authz-server docker/mock-https-authz-server/Dockerfile
bin/mock-https-authz-server: cmd/mock-https-authz-server/*.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $@ cmd/mock-https-authz-server/*.go

ifneq ($(shell go env GOBIN),)
  WKSCTL_INSTALL_PATH=$(shell go env GOBIN)
else
  WKSCTL_INSTALL_PATH=$(shell go env GOPATH)/bin
endif

install: bin/wksctl
	cp bin/wksctl "$(WKSCTL_INSTALL_PATH)"

lint:
	tools/go-lint

clean:
	$(SUDO) docker rmi $(IMAGE_NAMES) >/dev/null 2>&1 || true
	find docker test -type f -name "$(UPTODATE)" -delete
	rm -f $(BINARIES)

push:
	for IMAGE_NAME in $(IMAGE_NAMES); do \
		docker push $$IMAGE_NAME:$(IMAGE_TAG); \
	done

# We select which directory we want to descend into to not execute integration
# tests here.
unit-tests: generated
	go test -p 1 -v ./cmd/... ./pkg/...

# Tests running in containers
mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
mkfile_dir := $(dir $(mkfile_path))

container-tests: pkg/apis/wksprovider/machine/scripts/scripts_vfsdata.go pkg/apis/wksprovider/controller/manifests/manifests_vfsdata.go
	go test -count=1 ./test/container/...

integration-tests-container: bin/wksctl docker/controller/.uptodate
	IMAGE_TAG=$(IMAGE_TAG) go test -v -timeout 20m ./test/integration/container/...

FORCE:


docs-deps:
	pip3 install -r docs/requirements.txt

serve-docs: docs-deps
	mkdocs serve
