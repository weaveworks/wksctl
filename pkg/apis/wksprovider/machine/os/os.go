package os

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	existinginfrav1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	capeiresource "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/manifest"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/manifests"
	"k8s.io/api/apps/v1beta2"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const (
	sealedSecretVersion       = "v0.11.0"
	sealedSecretKeySecretName = "sealed-secrets-key"
	fluxSecretTemplate        = `apiVersion: v1
{{ if .SecretValue }}
data:
  identity: {{.SecretValue}}
{{ end }}
kind: Secret
metadata:
  name: flux-git-deploy
  namespace: {{.Namespace}}
type: Opaque`
)

var (
	pemKeys = []string{"certificate-authority", "client-certificate", "client-key"}
)

// SetupSeedNode installs Kubernetes on this machine, and store the provided
// manifests in the API server, so that the rest of the cluster can then be
// set up by the WKS controller.
func SetupSeedNode(o *capeios.OS, params capeios.SeedNodeParams) error {
	sp, updatedParams, err := createSecretPlan(o, params)
	if err != nil {
		return err
	}
	p, err := capeios.CreateSeedNodeSetupPlan(o, updatedParams)
	if err != nil {
		return err
	}
	if sp != nil {
		b := plan.NewBuilder()
		b.AddResource("install:secret-support", sp)
		b.AddResource("install:seed-node", p)
		plan, err := b.Plan()
		if err != nil {
			return err
		}
		p = &plan
	}
	return capeios.ApplyPlan(o, p)
}

// createSecretPlan constructs the seed node plan used to setup auth secrets
// prior to turning control over to wks-controller
func createSecretPlan(o *capeios.OS, params SeedNodeParams) (*plan.Plan, SeedNodeParams, error) {
	b = plan.NewBuilder()
	pemSecretResources, authConfigMap, authConfigManifest, err := processPemFilesIfAny(b, &cluster.Spec, params.ConfigDirectory, params.Namespace, params.SealedSecretKeyPath, params.SealedSecretCertPath)
	if err != nil {
		return nil, nil, err
	}
	if pemSecretResources == nil {
		return nil, params, nil
	}
	info := &capeios.AuthParams{pemSecretResources, authConfigMap, authConfigManifest}
	newParams := params
	newParams.AuthInfo = info
	p, err := b.Plan()
	if err != nil {
		return nil, nil, err
	}
	return p, newParams, nil
}

// processPemFilesIfAny reads the SealedSecret from the config
// directory, decrypts it using the GitHub deploy key, creates file
// resources for .pem files stored in the secret, and creates a SealedSecret resource
// for them that can be used by the machine actuator
func processPemFilesIfAny(builder *plan.Builder, providerSpec *capeiv1alpha3.ClusterSpec, configDir string, ns, privateKeyPath, certPath string) (map[string]*secretResourceSpec, *v1.ConfigMap, []byte, error) {
	if err := checkPemValues(providerSpec, privateKeyPath, certPath); err != nil {
		return nil, nil, nil, err
	}
	if providerSpec.Authentication == nil && providerSpec.Authorization == nil {
		// no auth specified
		return nil, nil, nil, nil
	}
	b := plan.NewBuilder()
	b.AddResource("create:pem-dir", &capeiresource.Dir{Path: object.String(capeios.PemDestDir)})
	b.AddResource("set-perms:pem-dir", &capeiresource.Run{Script: object.String(fmt.Sprintf("chmod 600 %s", capeios.PemDestDir))}, plan.DependOn("create:pem-dir"))
	privateKey, err := getPrivateKey(privateKeyPath)
	if err != nil {
		return nil, nil, nil, err
	}
	var authenticationSecretFileName, authorizationSecretFileName, authenticationSecretName, authorizationSecretName string
	var authenticationSecretManifest, authorizationSecretManifest, authenticationConfig, authorizationConfig []byte
	var decrypted map[string][]byte
	secretResources := map[string]*secretResourceSpec{}
	if providerSpec.Authentication != nil {
		authenticationSecretFileName = providerSpec.Authentication.SecretFile
		authenticationSecretManifest, decrypted, authenticationSecretName, authenticationConfig, err = processSecret(
			b, privateKey, configDir, authenticationSecretFileName, providerSpec.Authentication.URL)
		if err != nil {
			return nil, nil, nil, err
		}
		secretResources["authentication"] = &secretResourceSpec{
			secretName: authenticationSecretName,
			decrypted:  decrypted,
			resource:   &resource.KubectlApply{Namespace: object.String(ns), Manifest: authenticationSecretManifest, Filename: object.String(authenticationSecretName)}}
	}
	if providerSpec.Authorization != nil {
		authorizationSecretFileName = providerSpec.Authorization.SecretFile
		authorizationSecretManifest, decrypted, authorizationSecretName, authorizationConfig, err = processSecret(
			b, privateKey, configDir, authorizationSecretFileName, providerSpec.Authorization.URL)
		if err != nil {
			return nil, nil, nil, err
		}
		secretResources["authorization"] = &secretResourceSpec{
			secretName: authorizationSecretName,
			decrypted:  decrypted,
			resource:   &resource.KubectlApply{Namespace: object.String(ns), Manifest: authorizationSecretManifest, Filename: object.String(authorizationSecretName)}}
	}
	filePlan, err := b.Plan()
	if err != nil {
		log.Infof("Plan creation failed:\n%s\n", err)
		return nil, nil, nil, err
	}
	builder.AddResource("install:pem-files", &filePlan, plan.DependOn("install:config"))
	authConfigMap, authConfigMapManifest, err := createAuthConfigMapManifest(authenticationSecretName, authorizationSecretName,
		authenticationConfig, authorizationConfig)
	if err != nil {
		return nil, nil, nil, err
	}
	return secretResources, authConfigMap, authConfigMapManifest, nil
}

func getPrivateKey(privateKeyPath string) (*rsa.PrivateKey, error) {
	privateKeyBytes, err := getConfigFileContents(privateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "Could not read private key")
	}
	privateKeyData, err := keyutil.ParsePrivateKeyPEM(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := privateKeyData.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Private key file %q did not contain valid private key", privateKeyPath)
	}
	return privateKey, nil
}

func checkPemValues(providerSpec *capeiv1alpha3.ClusterSpec, privateKeyPath, certPath string) error {
	if privateKeyPath == "" || certPath == "" {
		if providerSpec.Authentication != nil || providerSpec.Authorization != nil {
			return errors.New("Encryption keys not specified; cannot process authentication and authorization specifications.")
		}
	}
	if (providerSpec.Authentication != nil && providerSpec.Authentication.SecretFile == "") ||
		(providerSpec.Authorization != nil && providerSpec.Authorization.SecretFile == "") {
		return errors.New("A secret must be specified to configure an authentication or authorization specification.")
	}
	return nil
}

func createAuthConfigMapManifest(authnSecretName, authzSecretName string, authnConfig, authzConfig []byte) (*v1.ConfigMap, []byte, error) {
	data := map[string]string{}
	storeIfNotEmpty(data, "authentication-secret-name", authnSecretName)
	storeIfNotEmpty(data, "authorization-secret-name", authzSecretName)
	storeIfNotEmpty(data, "authentication-config", string(authnConfig))
	storeIfNotEmpty(data, "authorization-config", string(authzConfig))
	cm := v1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "auth-config"},
		Data:       data,
	}
	manifest, err := yaml.Marshal(cm)
	if err != nil {
		return nil, nil, err
	}
	return &cm, manifest, nil
}

// Decrypts secret, adds plan resources to install files found inside, plus a kubeconfig file pointing to them.
// returns the sealed file contents, decrypted contents, secret name, kubeconfig, error if any
func processSecret(b *plan.Builder, key *rsa.PrivateKey, configDir, secretFileName, URL string) ([]byte, map[string][]byte, string, []byte, error) {
	// Read the file contents at configDir/secretFileName
	contents, err := getConfigFileContents(configDir, secretFileName)
	if err != nil {
		return nil, nil, "", nil, err
	}

	// Create a new YAML FrameReader from the given bytes
	fr := serializer.NewYAMLFrameReader(serializer.FromBytes(contents))
	// Create the secret to decode into
	ss := &ssv1alpha1.SealedSecret{}
	// Decode the Sealed Secret into the object
	// In the future, if we wish to support other kinds of secrets than SealedSecrets, we
	// can just change this to do .Decode(fr), and switch on the type
	if err := scheme.Serializer.Decoder().DecodeInto(fr, ss); err != nil {
		return nil, nil, "", nil, errors.Wrapf(err, "couldn't decode the file %q into a sealed secret", secretFileName)
	}

	fingerprint, err := crypto.PublicKeyFingerprint(&key.PublicKey)
	if err != nil {
		return nil, nil, "", nil, err
	}
	keys := map[string]*rsa.PrivateKey{fingerprint: key}

	codecs := scheme.Serializer.Codecs()
	if codecs == nil {
		return nil, nil, "", nil, fmt.Errorf("codecs must not be nil")
	}
	secret, err := ss.Unseal(*codecs, keys)
	if err != nil {
		return nil, nil, "", nil, errors.Wrap(err, "Could not unseal auth secret")
	}
	decrypted := map[string][]byte{}
	secretName := secret.Name
	for _, key := range pemKeys {
		fileContents, ok := secret.Data[key]
		if !ok {
			return nil, nil, "", nil, fmt.Errorf("Missing auth config value for: %q in secret %q", key, secretName)
		}
		resName := secretName + "-" + key
		fileName := filepath.Join(capeios.PemDestDir, secretName, key+".pem")
		b.AddResource("install:"+resName, &capeiresource.File{Content: string(fileContents), Destination: fileName}, plan.DependOn("set-perms:pem-dir"))
		decrypted[key] = fileContents
	}
	contextName := secretName + "-webhook"
	userName := secretName + "-api-server"
	config := &clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*clientcmdapi.Cluster{
			secretName: {
				CertificateAuthority: filepath.Join(capeios.PemDestDir, secretName, "certificate-authority.pem"),
				Server:               URL,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			userName: {
				ClientCertificate: filepath.Join(capeios.PemDestDir, secretName, "client-certificate.pem"),
				ClientKey:         filepath.Join(capeios.PemDestDir, secretName, "client-key.pem"),
			},
		},
		CurrentContext: contextName,
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  secretName,
				AuthInfo: userName,
			},
		},
	}
	authConfig, err := clientcmd.Write(*config)
	if err != nil {
		return nil, nil, "", nil, err
	}
	configResource := &capeiresource.File{Content: string(authConfig), Destination: filepath.Join(capeios.ConfigDestDir, secretName+".yaml")}
	b.AddResource("install:"+secretName, configResource, plan.DependOn("set-perms:pem-dir"))

	return contents, decrypted, secretName, authConfig, nil
}

func createSealedSecretKeySecretManifest(privateKey, cert, ns string) ([]byte, error) {
	secret := &v1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: sealedSecretKeySecretName, Namespace: "kube-system"},
		Type:       v1.SecretTypeOpaque,
	}
	secret.Data = map[string][]byte{}
	secret.StringData = map[string]string{}
	secret.StringData[v1.TLSPrivateKeyKey] = privateKey
	secret.StringData[v1.TLSCertKey] = cert
	return yaml.Marshal(secret)
}

// processDeployKey adds the encoded deploy key to the set of parameters used to configure flux
func processDeployKey(params map[string]string, gitDeployKeyPath string) error {
	if gitDeployKeyPath == "" {
		return nil
	}
	b64Key, err := readAndBase64EncodeKey(gitDeployKeyPath)
	if err != nil {
		return err
	}
	params["gitDeployKey"] = b64Key
	return nil
}

func readAndBase64EncodeKey(keypath string) (string, error) {
	content, err := ioutil.ReadFile(keypath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

func storeIfNotEmpty(vals map[string]string, key, value string) {
	if value != "" {
		vals[key] = value
	}
}

// getConfigFileContents reads a config manifest from a file in the config directory.
func getConfigFileContents(fileNameComponent ...string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(fileNameComponent...))
}

type secretResourceSpec struct {
	secretName string
	decrypted  capeiresource.SecretData
	resource   plan.Resource
}

func findManifest(dir, name string) (result string, err error) {
	err = filepath.Walk(dir,
		func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return nil // Other files may still be okay
			}
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			if info.Name() == name {
				result = path
				return filepath.SkipDir
			}
			return nil
		})
	if err != nil {
		result = ""
		return
	}
	if result == "" {
		err = fmt.Errorf("No %q manifest found in directory: %q", name, dir)
	}
	return
}

func findFluxManifest(dir string) (string, error) {
	return findManifest(dir, "flux.yaml")
}

func findControllerManifest(dir string) (string, error) {
	return findManifest(dir, "wks-controller.yaml")
}

func capiControllerManifest(controller capeios.ControllerParams, namespace, configDir string) ([]byte, error) {
	var file io.ReadCloser
	filepath, err := findManifest(configDir, "capi-controller.yaml")
	if err != nil {
		file, err = manifests.Manifests.Open("04_capi_controller.yaml")
	} else {
		file, err = os.Open(filepath)
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	manifestbytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	content, err := manifest.WithNamespace(serializer.FromBytes(manifestbytes), namespace)
	return content, err
}

func wksControllerManifest(controller capeios.ControllerParams, namespace, configDir string) ([]byte, error) {
	var manifestbytes []byte

	// The controller manifest is taken, in order:
	// 1. from the specified git repository checkout.
	// 2. from the YAML manifest built-in the binary.
	//
	// The controller image is, in priority order:
	// 1. controllerImageOverride provided on the apply command line.
	// 2. the image from the manifest if we have found a manifest in the git repository checkout.
	// 3. docker.io/weaveworks/wksctl-controller:version.ImageTag
	filepath, err := findControllerManifest(configDir)
	if err != nil {
		file, openErr := manifests.Manifests.Open("04_controller.yaml")
		if openErr != nil {
			return nil, openErr
		}
		manifestbytes, err = ioutil.ReadAll(file)
	} else {
		manifestbytes, err = ioutil.ReadFile(filepath)
	}
	if err != nil {
		return nil, err
	}
	content, err := manifest.WithNamespace(serializer.FromBytes(manifestbytes), namespace)
	if err != nil {
		return nil, err
	}
	return updateControllerImage(content, controller.ImageOverride)
}

const deployment = "Deployment"

// updateControllerImage replaces the controller image in the manifest and
// returns the updated manifest
func updateControllerImage(manifest []byte, controllerImageOverride string) ([]byte, error) {
	if controllerImageOverride == "" {
		return manifest, nil
	}
	d := &v1beta2.Deployment{}
	if err := yaml.Unmarshal(manifest, d); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal WKS controller's manifest")
	}
	if d.Kind != deployment {
		return nil, fmt.Errorf("invalid kind for WKS controller's manifest: expected %q but got %q", deployment, d.Kind)
	}
	var updatedController bool
	for i := 0; i < len(d.Spec.Template.Spec.Containers); i++ {
		if d.Spec.Template.Spec.Containers[i].Name == "controller" {
			d.Spec.Template.Spec.Containers[i].Image = controllerImageOverride
			updatedController = true
		}
	}
	if !updatedController {
		return nil, errors.New("failed to update WKS controller's manifest: container not found")
	}
	return yaml.Marshal(d)
}
