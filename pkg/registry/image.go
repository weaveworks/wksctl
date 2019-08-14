package registry

import (
	"fmt"
	"strings"
)

// Image represents the "coordinates" of a container image
// i.e.: REGISTRY[:PORT]/USER/NAME[:TAG]
// e.g.:
// - "quay.io/weaveworks/wksctl:latest"
// - "localhost:5000/test/busybox:v1.2.3"
// - "golang:1.10"
//
// See also:
// - https://github.com/moby/moby/blob/master/image/spec/v1.2.md#terminology
// - https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux_atomic_host/7/html/recommended_practices_for_container_development/naming
// - https://success.docker.com/article/what-is-the-correct-format-to-name-and-tag-dtr-repositories
//
type Image struct {
	Registry string // Host AND port
	User     string
	Name     string
	Tag      string
}

// NewImage parses the provided string representation of an image to return a registry.Image struct.
func NewImage(image string) (*Image, error) {
	if strings.TrimSpace(image) == "" {
		return nil, fmt.Errorf("invalid image: '%v'", image)
	}
	parts := strings.Split(image, "/")
	registry, user, name, tag := "", "", "", ""
	switch len(parts) {
	case 1:
		name = parts[0]
	case 2:
		user, name = parts[0], parts[1]
	case 3:
		registry, user, name = parts[0], parts[1], parts[2]
	default:
		return nil, fmt.Errorf("invalid image: '%v'", image)
	}
	nameAndTag := strings.Split(name, ":")
	switch len(nameAndTag) {
	case 1:
		break
	case 2:
		name, tag = nameAndTag[0], nameAndTag[1]
	default:
		return nil, fmt.Errorf("invalid image: '%v'", image)
	}
	return &Image{Registry: registry, User: user, Name: name, Tag: tag}, nil
}

// String returns the string representation of this Image struct.
func (image Image) String() string {
	var builder strings.Builder
	if image.Registry != "" {
		builder.WriteString(image.Registry)
		builder.WriteString("/")
	}
	if image.User != "" {
		builder.WriteString(image.User)
		builder.WriteString("/")
	}
	builder.WriteString(image.Name)
	if image.Tag != "" {
		builder.WriteString(":")
		builder.WriteString(image.Tag)
	}
	return builder.String()
}

// CommandsToRetagAs returns the docker pull, docker tag, and docker push
// commands required to "retag" this (source) container image under a different
// registry, user, name, and/or tag, as specified by the provided (destination)
// image.
func (image Image) CommandsToRetagAs(destImage Image) []string {
	return []string{
		fmt.Sprintf("docker pull %s", image),
		fmt.Sprintf("docker tag %s %s", image, destImage),
		fmt.Sprintf("docker push %s", destImage),
	}
}

// ByCoordinate allows you to sort registry.Image arrays.
type ByCoordinate []Image

func (a ByCoordinate) Len() int      { return len(a) }
func (a ByCoordinate) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByCoordinate) Less(i, j int) bool {
	comparison := strings.Compare(a[i].Registry, a[j].Registry)
	if comparison != 0 {
		return comparison < 0
	}
	comparison = strings.Compare(a[i].User, a[j].User)
	if comparison != 0 {
		return comparison < 0
	}
	comparison = strings.Compare(a[i].Name, a[j].Name)
	if comparison != 0 {
		return comparison < 0
	}
	return strings.Compare(a[i].Tag, a[j].Tag) < 0
}
