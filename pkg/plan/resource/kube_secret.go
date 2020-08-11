package resource

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// KubeSecret writes secrets to the filesystem where they can be picked up by daemons
type KubeSecret struct {
	base

	// SecretName is the name of the secret to read
	SecretName string `structs:"secretName"`
	// Checksum contains the sha256 checksum of the secret data
	Checksum [sha256.Size]byte `structs:"checksum"`
	// DestinationDirectory is the location in which to write stored file data
	DestinationDirectory string `structs:"destinationDirectory"`
	// SecretData holds the actual secret contents -- not serialized
	SecretData SecretData `structs:"-" plan:"hide"`
	// FileNameTransform transforms a secret key into the file name for its contents
	FileNameTransform func(string) string
}

// SecretData maps names to values as in Kubernetes v1.Secret
type SecretData map[string][]byte

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

// NewKubeSecretResource creates a new object from secret data
func NewKubeSecretResource(secretName string, secretData SecretData, destinationDirectory string, fileNameTransform func(string) string) (*KubeSecret, error) {
	return &KubeSecret{
		SecretName:           secretName,
		Checksum:             sha256.Sum256(flattenMap(secretData)),
		DestinationDirectory: destinationDirectory,
		SecretData:           secretData,
		FileNameTransform:    fileNameTransform,
	}, nil
}

// State implements plan.Resource.
func (ks *KubeSecret) State() plan.State {
	return plan.State(map[string]interface{}{"checksum": ks.Checksum})
}

func (ks *KubeSecret) QueryState(ctx context.Context, runner plan.Runner) (plan.State, error) {
	data := ks.SecretData
	for fname := range data {
		path := filepath.Join(ks.DestinationDirectory, ks.FileNameTransform(fname))
		exists, err := fileExists(ctx, runner, path)
		if err != nil {
			return nil, err
		}
		if !exists {
			return plan.EmptyState, nil
		}
		contents, err := runner.RunCommand(ctx, fmt.Sprintf("cat %s", path), nil)
		if err != nil {
			return nil, err
		}
		data[fname] = []byte(contents)
	}
	return plan.State(map[string]interface{}{"checksum": sha256.Sum256(flattenMap(data))}), nil
}

func fileExists(ctx context.Context, runner plan.Runner, path string) (bool, error) {
	result, err := runner.RunCommand(ctx, fmt.Sprintf("[ -f %s ] && echo 'yes' || true", path), nil)
	if err != nil {
		return false, err
	}
	return result == "yes", nil
}

// Apply implements plan.Resource.
func (ks *KubeSecret) Apply(ctx context.Context, runner plan.Runner, diff plan.Diff) (bool, error) {
	for fname, contents := range ks.SecretData {
		err := scripts.WriteFile(ctx, contents, filepath.Join(ks.DestinationDirectory, ks.FileNameTransform(fname)), 0600, runner)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// Undo implements plan.Resource.
func (ks *KubeSecret) Undo(ctx context.Context, runner plan.Runner, current plan.State) error {
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
	_, err := runner.RunCommand(ctx, fmt.Sprintf("rm -f %s/%s", ks.DestinationDirectory, sb.String()), nil)
	return err
}
