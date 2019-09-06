package resource

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
)

// KubectlApply is a resource applying the provided manifest.
// It doesn't realise any state, Apply will always apply the manifest.
type KubectlApply struct {
	base

	// Filename is the remote manifest file name.
	// Only provide this if you do NOT provide ManifestPath or ManifestURL.
	Filename fmt.Stringer `structs:"filename"`
	// Manifest is the actual YAML/JSON content of the manifest to apply.
	// If this is provided, then there is no need to provide ManifestPath, but
	// Filename should be provided in order to name the remote manifest file.
	Manifest []byte `structs:"manifest"`
	// ManifestPath is the path to the manifest to apply.
	// If this is provided, then there is no need to provide Manifest.
	ManifestPath fmt.Stringer `structs:"manifestPath"`
	// ManifestURL is the URL of a remote manifest; if specified,
	// neither Filename, Manifest, nor ManifestPath should be specified.
	ManifestURL fmt.Stringer `structs:"manifestURL"`
	// WaitCondition, if not empty, makes Apply() perform "kubectl wait --for=<value>" on the resource.
	Namespace fmt.Stringer `structs:"namespace"`
	// OpaqueManifest is an alternative to Manifest for a resource to
	// apply whose content should not be exposed in a serialized plan.
	// If this is provided, then there is no need to provide
	// ManifestPath, but Filename should be provided in order to name
	// the remote manifest file.
	OpaqueManifest []byte `structs:"-" plan:"hide"`
	// ManifestPath is the path to the manifest to apply.
	// If this is provided, then there is no need to provide Manifest.
	// For example, waiting for "condition=established" is required after creating a CRD - see issue #530.
	WaitCondition string `structs:"afterApplyWaitsFor"`
}

func str(v fmt.Stringer) string {
	if v == nil {
		return ""
	}
	return v.String()
}

var _ plan.Resource = plan.RegisterResource(&KubectlApply{})

// State implements plan.Resource.
func (ka *KubectlApply) State() plan.State {
	return toState(ka)
}

func (ka *KubectlApply) content() ([]byte, error) {
	if ka.Manifest != nil {
		return ka.Manifest, nil
	}

	if ka.OpaqueManifest != nil {
		return ka.OpaqueManifest, nil
	}

	if url := str(ka.ManifestURL); url != "" {
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return ioutil.ReadAll(resp.Body)
	}

	if path := str(ka.ManifestPath); path != "" {
		return ioutil.ReadFile(path)
	}

	return nil, errors.New("no content provided")
}

func writeTempFile(r plan.Runner, c []byte, fname string) (string, error) {
	pathDirty, err := r.RunCommand(fmt.Sprintf("mktemp -t %sXXXXXXXXXX", fname), nil)
	if err != nil {
		return "", errors.Wrap(err, "mktemp")
	}
	path := strings.Trim(pathDirty, "\n")

	if err := scripts.WriteFile(c, path, 0660, r); err != nil {
		// Try to delete the temp file.
		if _, rmErr := r.RunCommand(fmt.Sprintf("rm -vf %q", path), nil); rmErr != nil {
			log.WithField("path", path).Errorf("failed to clean up the temp file: %v", rmErr)
		}

		return "", errors.Wrap(err, "WriteFile")
	}

	return path, nil
}

// Apply performs a "kubectl apply" as specified in the receiver.
func (ka *KubectlApply) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	var content string

	// Get the manifest content.
	c, err := ka.content()
	if err != nil {
		return false, err
	}

	if str(ka.Namespace) != "" {
		content, err = manifest.WithNamespace(string(c), str(ka.Namespace))
		if err != nil {
			return false, err
		}
	}
	if content != "" {
		c = []byte(content)
	}

	if err := kubectlApply(runner, kubectlApplyArgs{
		Content:       c,
		WaitCondition: ka.WaitCondition,
	}, str(ka.Filename)); err != nil {
		return false, err
	}

	return true, nil
}

type kubectlApplyArgs struct {
	// Content is the YAML manifest to be applied. Must be non-empty.
	Content []byte
	// WaitCondition, if non-empty, makes kubectlApply do "kubectl wait --for=<value>" on the applied resource.
	WaitCondition string
}

func kubectlApply(r plan.Runner, args kubectlApplyArgs, fname string) error {
	// Write the manifest content to the remote filesystem.
	path, err := writeTempFile(r, args.Content, fname)
	if err != nil {
		return errors.Wrap(err, "writeTempFile")
	}
	defer r.RunCommand(fmt.Sprintf("rm -vf %q", path), nil)

	// Run kubectl apply.
	if err := kubectlRemoteApply(path, r); err != nil {
		return errors.Wrap(err, "kubectl apply")
	}

	// Run kubectl wait, if requested.
	if args.WaitCondition != "" {
		cmd := fmt.Sprintf("kubectl wait --for=%q -f %q", args.WaitCondition, path)
		if _, err := r.RunCommand(withoutProxy(cmd), nil); err != nil {
			return errors.Wrap(err, "kubectl wait")
		}
	}

	// Great success!
	return nil
}

func kubectlRemoteApply(remoteURL string, runner plan.Runner) error {
	cmd := fmt.Sprintf("kubectl apply -f %q", remoteURL)

	if stdouterr, err := runner.RunCommand(withoutProxy(cmd), nil); err != nil {
		log.WithField("stdouterr", stdouterr).WithField("URL", remoteURL).Debug(fmt.Sprintf("failed to apply Kubernetes manifest"))
		return errors.Wrapf(err, "failed to apply manifest %q", remoteURL)
	}
	return nil
}
