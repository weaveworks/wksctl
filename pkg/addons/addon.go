package addons

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ghodss/yaml"
	"github.com/google/go-jsonnet"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/addons/assets"
	"github.com/weaveworks/wksctl/pkg/qjson"
	"github.com/weaveworks/wksctl/pkg/registry"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
)

var mutex *sync.Mutex = &sync.Mutex{}

const (
	descriptor = "addon.json"
)

// addonKind specifies what type of addon has been defined.
type addonKind string

const (
	// AddonKindJsonnet is an addon written with jsonnet.
	addonKindJsonnet addonKind = ""
	// AddonKindYAML is addon composed of YAML manifests.
	addonKindYAML addonKind = "yaml"
)

// ParamKind specifies the input parameter type.
type ParamKind string

const (
	// ParamKindString is a simple string parameter.
	ParamKindString ParamKind = ""
	// ParamKindFile is base64-encoded file content.
	ParamKindFile ParamKind = "file"
)

// ParamError describes an error on the input parameters.
type ParamError struct {
	Param   string
	Message string
}

func (e *ParamError) Error() string {
	return e.Message
}

func newParamError(param, message string) error {
	return &ParamError{
		Param:   param,
		Message: message,
	}
}

func newParamErrorf(param, format string, args ...interface{}) error {
	return &ParamError{
		Param:   param,
		Message: fmt.Sprintf(format, args...),
	}
}

// Param is a input parameter for addon configuration.
type Param struct {
	// Name of the parameter
	Name string
	// Target is the TLA name in the jsonnet file. Defaults to Name.
	Target       string
	Kind         ParamKind
	Required     bool
	DefaultValue string
	Description  string
}

func resolvePath(base, s string) string {
	if path.IsAbs(s) {
		return s
	}

	return path.Join(base, s)
}

func (p *Param) value(config *BuildOptions, input string) (string, error) {
	switch p.Kind {
	case ParamKindFile:
		path := resolvePath(config.BasePath, input)
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(content), nil
	case ParamKindString:
		fallthrough
	default:
		return input, nil
	}
}

// output is the jsonnet evaluation mode
type output string

const (
	outputSingle   output = "single"
	outputMultiple output = "multiple"
)

// Addon is a piece of software that can be installed on a Kubernetes cluster.
type Addon struct {
	Kind        addonKind
	Category    string
	Name        string
	Description string
	Params      []Param

	ShortName string // The directory name in addons/
	// Entrypoint is either:
	//  - the jsonnet file to execute to build manifests for jsonnet addons.
	//  - a multi-document YAML file for YAML addons.
	EntryPoint string
	// Jsonnet file to execute to list images. The result is an array of image
	// strings. eg.
	// [
	//   "quay.io/coreos/addon-resizer:1.0",
	//   "quay.io/prometheus/alertmanager:v0.15.3",
	//   "quay.io/coreos/configmap-reload:v0.0.1",
	//   "grafana/grafana:5.2.4"
	// ]
	ListImagesEntryPoint string
	OutputMode           output // How to evaluate the jsonnet script. Default to Single.
}

func addonDescriptor(shortName string) string {
	return "/" + shortName + "/" + descriptor
}

// Get returns the Addon with the corresponding shortName.
func Get(shortName string) (Addon, error) {
	desc, err := assets.Assets.Open(addonDescriptor(shortName))
	if err != nil {
		return Addon{}, fmt.Errorf("addon: couldn't find %s", shortName)
	}

	addon := Addon{
		ShortName: shortName,
	}
	if err := json.NewDecoder(desc).Decode(&addon); err != nil {
		return addon, fmt.Errorf("addon: couldn't parse descriptor for %s", shortName)
	}

	for i := range addon.Params {
		param := &addon.Params[i]
		if param.Target == "" {
			param.Target = param.Name
		}
	}

	return addon, nil
}

// List returns the list of known addons.
func List() []Addon {
	var addons []Addon

	root, _ := assets.Assets.Open("/")
	files, _ := root.Readdir(-1)
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if f.Name() == "vendor" {
			continue
		}

		addon, err := Get(f.Name())
		if err != nil {
			log.Debug(err)
			continue
		}

		addons = append(addons, addon)
	}

	return addons
}

// Param returns the named Param.
func (a *Addon) Param(name string) *Param {
	for i := range a.Params {
		param := &a.Params[i]
		if param.Name == name {
			return param
		}
	}
	return nil
}

// HasParam returns true if the addon has name as input parameter.
func (a *Addon) HasParam(name string) bool {
	return a.Param(name) != nil
}

// BuildOptions holds some options for Build.
type BuildOptions struct {
	// Base path against which relative paths should be resolved (used for File
	// parameters).
	BasePath string
	// Output directory is the where the addon manifests should be written to. If
	// not given, the current working directory will be used.
	OutputDirectory string
	Params          map[string]string
	// ImageRepository indicates container images should be sourced from this
	// registry instead of their default one(s).
	ImageRepository string
	YAML            bool
}

func extension(config *BuildOptions) string {
	if config.YAML {
		return ".yaml"
	}
	return ".json"
}

// ValidateOptions validates that the given BuildOptions are valid for this
// addon.
func (a *Addon) ValidateOptions(config *BuildOptions) error {
	// Check whether the provided params are defined by the addon,
	for k := range config.Params {
		param := a.Param(k)
		if param == nil {
			return newParamErrorf(k, "addon: unknown parameter '%s'", k)
		}
	}

	// Ensure required parameters are indeed provided.
	for i := range a.Params {
		param := &a.Params[i]
		if !param.Required {
			continue
		}
		if _, ok := config.Params[param.Name]; !ok {
			return newParamErrorf(param.Name, "addon: parameter '%s' is required but not provided", param.Name)
		}
	}

	// Check we can compute the param value, eg. when we need to read a file, that
	// the file exists and is readable.
	for k, v := range config.Params {
		param := a.Param(k)
		_, err := param.value(config, v)
		if err != nil {
			return newParamError(k, err.Error())
		}
	}

	return nil
}

func (a *Addon) absEntryPoint(entry string) string {
	return "/" + a.ShortName + "/" + entry
}

func (a *Addon) evaluate(vm *jsonnet.VM, config *BuildOptions, script string) (map[string]string, error) {
	output := make(map[string]string)

	switch a.OutputMode {
	case outputMultiple:
		result, err := vm.EvaluateSnippetMulti(a.EntryPoint, script)
		if err != nil {
			return nil, err
		}
		for k, v := range result {
			output[filepath.Join(config.OutputDirectory, k+extension(config))] = v
		}
	case outputSingle:
		fallthrough
	default:
		j, err := vm.EvaluateSnippet(a.EntryPoint, script)
		if err != nil {
			return nil, err
		}
		output[filepath.Join(config.OutputDirectory, a.ShortName+extension(config))] = j
	}

	return output, nil
}

func makeVM() *jsonnet.VM {
	vm := jsonnet.MakeVM()

	importer := newVFSImporter()
	importer.searchPaths = []string{"/", "/vendor"}
	importer.assets = assets.Assets
	vm.Importer(importer)

	return vm
}

func (a *Addon) buildJsonnet(config BuildOptions) ([]string, error) {
	vm := makeVM()

	contents, err := assets.ReadAll(a.absEntryPoint(a.EntryPoint))
	if err != nil {
		return nil, err
	}

	// If the addon exposes it, we can override the repository of container images.
	if config.ImageRepository != "" && a.HasParam("imageRepository") {
		config.Params["imageRepository"] = config.ImageRepository
	}

	for k, v := range config.Params {
		param := a.Param(k)
		if param == nil {
			return nil, fmt.Errorf("addon: unknown parameter '%s'", k)
		}
		value, err := param.value(&config, v)
		if err != nil {
			return nil, err
		}
		vm.TLAVar(param.Target, value)
	}

	output, err := a.evaluate(vm, &config, string(contents))
	if err != nil {
		return nil, err
	}

	for filename, value := range output {
		if config.YAML {
			data, err := yaml.JSONToYAML([]byte(value))
			if err != nil {
				return nil, err
			}
			value, err = manifest.WithNamespace(string(data), "wkp-addons")
			if err != nil {
				return nil, err
			}
		}
		if err := ioutil.WriteFile(filename, []byte(value), 0660); err != nil {
			return nil, err
		}
	}

	// Build the list of written manifests.
	manifests := make([]string, len(output))
	i := 0
	for k := range output {
		manifests[i] = k
		i++
	}

	// Sorting is important as it's used to order creation of Kubernetes objects
	// by, eg. the prometheus operator with 00namespace.yaml.
	sort.Strings(manifests)

	return manifests, nil
}

func (a *Addon) buildYAML(config BuildOptions) ([]string, error) {
	manifests, err := assets.ReadAll(a.absEntryPoint(a.EntryPoint))
	if err != nil {
		return nil, err
	}

	// Rewrite the images references
	var objects object
	if config.ImageRepository != "" {
		var err error

		objects, err = newObjectFromYAML([]byte(manifests))
		if err != nil {
			return nil, err
		}
		objects = transformList(objects, withImageRepository(config.ImageRepository))
	}

	var output []byte
	if !objects.IsEmpty() {
		if config.YAML {
			output, err = objects.toYAML()
		} else {
			output, err = objects.toJSON()
		}
	} else {
		if config.YAML {
			output = []byte(manifests)
		} else {
			output, err = yaml.YAMLToJSON([]byte(manifests))
		}
	}

	if err != nil {
		return nil, err
	}

	filename := filepath.Join(config.OutputDirectory, a.EntryPoint)
	if err := ioutil.WriteFile(filename, output, 0660); err != nil {
		return nil, err
	}
	return []string{
		filename,
	}, nil
}

// Build builds the addon manifests and write them to disk. It returns the list
// of written files.
func (a *Addon) Build(config BuildOptions) ([]string, error) {
	switch a.Kind {
	case addonKindJsonnet:
		return a.buildJsonnet(config)
	case addonKindYAML:
		return a.buildYAML(config)
	default:
		return nil, fmt.Errorf("unknown addon kind '%s'", a.Kind)
	}
}

func (a *Addon) listImagesFromManifest() ([]registry.Image, error) {
	// First, get sample JSON Kubernetes manifests for this addon:
	tmpDir, err := ioutil.TempDir("", "wksctl-list-images")
	if err != nil {
		return nil, err
	}

	manifests, err := a.autoBuild(BuildOptions{OutputDirectory: tmpDir, YAML: false})
	if err != nil {
		return nil, fmt.Errorf("failed to auto-build addon %s: %v", a.Name, err)
	}

	var images []registry.Image
	for _, manifest := range manifests {
		// Extract the container images from this Kubernetes manifest:
		jsonBytes, err := ioutil.ReadFile(manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to read addon manifest: %v", err)
		}

		imageStrings, err := qjson.CollectStrings("spec.containers.#.image", jsonBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to extract images from addon %s: %v", a.Name, err)
		}

		// Parse and collect all the extracted container images:
		for _, imageString := range imageStrings {
			image, err := registry.NewImage(imageString)
			if err != nil {
				return nil, fmt.Errorf("failed to parse image %s from addon %s: %v", imageString, a.Name, err)
			}
			images = append(images, *image)
		}
	}

	// Cleanup the generated manifests.
	os.RemoveAll(tmpDir)

	return images, nil
}

func (a *Addon) listImagesFromScript() ([]registry.Image, error) {
	vm := makeVM()

	script, err := assets.ReadAll(a.absEntryPoint(a.ListImagesEntryPoint))
	if err != nil {
		return nil, err
	}

	output, err := vm.EvaluateSnippet(a.ListImagesEntryPoint, string(script))
	if err != nil {
		return nil, err
	}

	var imageStrings []string
	if err := json.Unmarshal([]byte(output), &imageStrings); err != nil {
		return nil, err
	}

	var images []registry.Image
	for _, desc := range imageStrings {
		image, err := registry.NewImage(desc)
		if err != nil {
			return nil, err
		}
		images = append(images, *image)
	}

	return images, nil
}

// ListImages lists all container images required for this addon to run.
func (a *Addon) ListImages() ([]registry.Image, error) {
	if a.ListImagesEntryPoint == "" {
		return a.listImagesFromManifest()
	}
	return a.listImagesFromScript()
}

// autoBuild builds the addon manifests, automatically providing "dummy" values
// to required parameters. This is typically useful when trying to generate a
// draft Kubernetes manifest for this addon.
// N.B.: The provided configuration can be empty, and if not, will override
// whatever is automatically generated.
func (a *Addon) autoBuild(config BuildOptions) ([]string, error) {
	params := map[string]string{}

	// Temporary file used to automatically fill parameters of kind "file".
	// We need to keep it around until the Jsonnet VM has evaluated the addon:
	tmpFile, err := ioutil.TempFile("", "file_autovalue")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	for _, param := range a.Params {
		// Try to automatically provide required parameters:
		if param.Required {
			switch param.Kind {
			case ParamKindFile:
				params[param.Name] = tmpFile.Name()
			case ParamKindString:
				params[param.Name] = "string_autovalue"
			default:
				return nil, fmt.Errorf("automated filling of parameter %v of kind %v is not implemented", param.Name, param.Kind)
			}
		}
	}

	// Always keep & overwrite with provided parameters:
	for k, v := range config.Params {
		params[k] = v
	}

	config.Params = params
	return a.Build(config)
}
