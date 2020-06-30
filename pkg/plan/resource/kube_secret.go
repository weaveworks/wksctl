package resource

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
	"github.com/weaveworks/wksctl/pkg/plan"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubeSecret is a resource that reads a value out of a secret and writes it to the filesystem. It
// can only be created when running in code deployed within the cluster because we want to store the
// hash of the secret data in the Resource before the Plan is run so we can compare it against a later
// version of the Plan when the secret is updated.
type KubeSecret struct {
	base

	// SecretName is the name of the secret to read
	SecretName string `structs:"secretName"`
	// Checksum contains the sha256 checksum of the secret data
	Checksum [sha256.Size]byte `structs:"checksum"`
	// DestinationDirectory is the location in which to write stored file data
	DestinationDirectory string `structs:"destinationDirectory"`
	// SecretData holds the actual secret contents -- not serialized
	SecretData map[string][]byte `structs:"-" plan:"hide"`
	// FileNameTransform transforms a secret key into the file name for its contents
	FileNameTransform func(string) string
}

var (
	_ plan.Resource = plan.RegisterResource(&KubeSecret{})
)

func flattenMap(m map[string][]byte) []byte {
	items := []string{}
	for key, val := range m {
		items = append(items, fmt.Sprintf("%s:%v", key, val))
	}
	sort.Strings(items)
	return []byte(strings.Join(items, ","))
}

func NewKubeSecretResource(secretName, destinationDirectory, ns string, fileNameTransform func(string) string) (*KubeSecret, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create Kubernetes client set: %v", err)
	}
	client := clientSet.CoreV1().Secrets(ns)
	secret, err := client.Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		// No secret present
		return nil, nil
	}
	return &KubeSecret{
		SecretName:           secretName,
		Checksum:             sha256.Sum256(flattenMap(secret.Data)),
		DestinationDirectory: destinationDirectory,
		SecretData:           secret.Data,
		FileNameTransform:    fileNameTransform,
	}, nil
}

// State implements plan.Resource.
func (ks *KubeSecret) State() plan.State {
	return plan.State(map[string]interface{}{"checksum": ks.Checksum})
}

func (ks *KubeSecret) QueryState(runner plan.Runner) (plan.State, error) {
	data := ks.SecretData
	for fname := range data {
		path := filepath.Join(ks.DestinationDirectory, ks.FileNameTransform(fname))
		exists, err := fileExists(runner, path)
		if err != nil {
			return nil, err
		}
		if !exists {
			return plan.EmptyState, nil
		}
		contents, err := runner.RunCommand(fmt.Sprintf("cat %s", path), nil)
		if err != nil {
			return nil, err
		}
		data[fname] = []byte(contents)
	}
	return plan.State(map[string]interface{}{"checksum": sha256.Sum256(flattenMap(data))}), nil
}

func fileExists(runner plan.Runner, path string) (bool, error) {
	result, err := runner.RunCommand(fmt.Sprintf("[ -f %s ] && echo 'yes' || true", path), nil)
	if err != nil {
		return false, err
	}
	return result == "yes", nil
}

// Apply implements plan.Resource.
func (ks *KubeSecret) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	for fname, contents := range ks.SecretData {
		err := scripts.WriteFile(contents, filepath.Join(ks.DestinationDirectory, ks.FileNameTransform(fname)), 0600, runner)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// Undo implements plan.Resource.
func (ks *KubeSecret) Undo(runner plan.Runner, current plan.State) error {
	if len(ks.SecretData) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("{")
	shouldWriteComma := false
	for filename := range ks.SecretData {
		if shouldWriteComma {
			sb.WriteString(",")
		} else {
			shouldWriteComma = true
		}
		sb.WriteString(ks.FileNameTransform(filename))
	}
	sb.WriteString("}")
	_, err := runner.RunCommand(fmt.Sprintf("rm -f %s/%s", ks.DestinationDirectory, sb.String()), nil)
	return err
}
