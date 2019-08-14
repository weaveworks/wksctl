package addons

import (
	"fmt"
	"strings"
)

type transform func(object) object

func printObject(o object) {
	json, err := o.toJSON()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(json))
}

func list(items []object) object {
	return object{
		"apiVersion": "v1",
		"kind":       "List",
		"items":      items,
	}
}

func transformList(l object, transforms ...transform) object {
	items, err := l.GetObjectArray("items")
	if err != nil {
		fmt.Println(err)
	}

	var transformedItems []object
	for _, item := range items {
		for _, t := range transforms {
			item = t(item)
		}
		transformedItems = append(transformedItems, item)
	}

	n := list(transformedItems)
	return n
}

func forEachContainer(o object, cb func(container object)) {
	paths := []string{
		"spec.template.spec.initContainers",
		"spec.template.spec.containers",
		"spec.jobTemplate.spec.template.spec.initContainers",
		"spec.jobTemplate.spec.template.spec.containers",
	}
	for _, path := range paths {
		containers, err := o.GetObjectArray(path)
		if err != nil {
			continue
		}
		for _, container := range containers {
			cb(container)
		}
	}

}

func withImageRepository(repository string) transform {
	return func(o object) object {
		forEachContainer(o, func(container object) {
			image, err := container.GetString("image")
			if err != nil {
				return
			}
			updatedImage, err := UpdateImage(image, repository)
			if err != nil {
				return
			}
			container.SetString("image", updatedImage)
		})
		return o
	}
}

// UpdateImage updates the provided container image's fully-qualified name with
// the provided repository.
func UpdateImage(image, repository string) (string, error) {
	if repository == "" {
		return image, nil
	}
	ref, err := parseImageReference(image)
	if err != nil {
		return "", err
	}

	// e.g.: "host:port/org" -> {"host:port", "org"}
	repositoryParts := strings.Split(repository, "/")
	if len(repositoryParts) > 2 {
		return "", fmt.Errorf("Invalid repository. Expected: \"host:port\" or \"host:port/org\" but got: %s", repository)
	}
	ref.Domain = repositoryParts[0]
	if len(repositoryParts) == 2 {
		// Override the organisation with the one provided in the repository:
		ref.Organisation = repositoryParts[1]
	}

	return ref.String(), nil
}
