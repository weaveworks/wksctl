package manifest

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/weaveworks/libgitops/pkg/serializer"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	DefaultNamespace = `weavek8sops`

	corev1Version = "v1"
	listKind      = "List"
	namespaceKind = "Namespace"
)

var DefaultAddonNamespaces = map[string]string{"weave-net": "kube-system"}

func WithNamespace(fileOrString, namespace string) ([]byte, error) {
	// Set up the readcloser from either a file, or from the given string
	var rc io.ReadCloser
	if isFile(fileOrString) {
		rc = serializer.FromFile(fileOrString)
	} else {
		rc = serializer.FromBytes([]byte(fileOrString))
	}

	// Create a FrameReader and FrameWriter, using YAML document separators
	// The FrameWriter will write into buf
	fr := serializer.NewYAMLFrameReader(rc)
	buf := new(bytes.Buffer)
	fw := serializer.NewYAMLFrameWriter(buf)

	// Read all frames from the FrameReader
	frames, err := serializer.ReadFrameList(fr)
	if err != nil {
		return nil, err
	}

	// If namespace is "", just write all the read frames to buf through the framewriter, and exit
	if namespace == "" {
		if err := serializer.WriteFrameList(fw, frames); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}

	// Loop through all the frames
	for _, frame := range frames {
		// Parse the given frame's YAML. JSON also works
		obj, err := kyaml.Parse(string(frame))
		if err != nil {
			return nil, err
		}

		// Get the TypeMeta of the given object
		meta, err := obj.GetMeta()
		if err != nil {
			return nil, err
		}

		// Use special handling for the v1.List, as we need to traverse each item in the .items list
		// Otherwise, just run setNamespaceOnObject for the parsed object
		if meta.APIVersion == corev1Version && meta.Kind == listKind {
			// Visit each item under .items
			if err := visitElementsForPath(obj, func(item *kyaml.RNode) error {
				// Set namespace on the given item
				return setNamespaceOnObject(item, namespace)

			}, "items"); err != nil {
				return nil, err
			}

		} else {
			// Set namespace on the given object
			if err := setNamespaceOnObject(obj, namespace); err != nil {
				return nil, err
			}
		}

		// Convert the object to string, and write it to the FrameWriter
		str, err := obj.String()
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(str)); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func setNamespaceOnObject(obj *kyaml.RNode, namespace string) error {
	// Get the TypeMeta of the given object
	meta, err := obj.GetMeta()
	if err != nil {
		return err
	}

	// The default namespaceFilter sets the "namespace" field (on the metadata object)
	// to the desired namespace
	namespaceFilter := setNamespaceFilter(namespace)
	// However, if the given object IS a Namespace, we set the "name" field to the desired
	// namespace name instead.
	if meta.APIVersion == corev1Version && meta.Kind == namespaceKind {
		namespaceFilter = kyaml.SetField("name", kyaml.NewScalarRNode(namespace))
	}

	// Lookup and create .metadata (if it doesn't exist), and set its
	// namespace field to the desired value
	err = obj.PipeE(
		kyaml.LookupCreate(kyaml.MappingNode, "metadata"),
		namespaceFilter,
	)
	if err != nil {
		return err
	}

	// Visit .subjects (if it exists), and traverse its elements, setting
	// the namespace field on each item
	return visitElementsForPath(obj, func(node *kyaml.RNode) error {
		return node.PipeE(setNamespaceFilter(namespace))
	}, "subjects")
}

func visitElementsForPath(obj *kyaml.RNode, fn func(node *kyaml.RNode) error, paths ...string) error {
	list, err := obj.Pipe(kyaml.Lookup(paths...))
	if err != nil {
		return err
	}
	return list.VisitElements(fn)
}

func setNamespaceFilter(ns string) kyaml.FieldSetter {
	return kyaml.SetField("namespace", kyaml.NewScalarRNode(ns))
}

func isFile(fileOrString string) bool {
	_, err := os.Stat(fileOrString)
	if err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), "file name too long")) {
		return false
	}
	return true
}
