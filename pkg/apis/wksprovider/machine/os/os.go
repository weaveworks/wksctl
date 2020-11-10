package os

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	ssv1alpha1 "github.com/bitnami-labs/sealed-secrets/pkg/apis/sealed-secrets/v1alpha1"
	"github.com/bitnami-labs/sealed-secrets/pkg/crypto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	existinginfrav1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	capeiresource "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/scheme"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/specs"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
	"github.com/weaveworks/libgitops/pkg/serializer"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/keyutil"
	"sigs.k8s.io/yaml"
)

var (
	pemKeys = []string{"certificate-authority", "client-certificate", "client-key"}
)

// SetupSeedNode installs Kubernetes on this machine, and store the provided
// manifests in the API server, so that the rest of the cluster can then be
// set up by the WKS controller.
func SetupSeedNode(o *capeios.OS, params capeios.SeedNodeParams) error {
	ctx := context.Background()
	sp, updatedParams, err := createSecretPlan(o, params)
	if err != nil {
		return err
	}
	updatedParams, err = createMachinePoolInfo(updatedParams)
	if err != nil {
		return err
	}
	p, err := capeios.CreateSeedNodeSetupPlan(ctx, o, updatedParams)
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
	return capeios.ApplyPlan(ctx, o, p)
}

// createMachinePoolInfo turns the specified machines into a connection pool
// that can be used to contact the machines
func createMachinePoolInfo(params capeios.SeedNodeParams) (capeios.SeedNodeParams, error) {
	_, eic, err := specs.ParseCluster(ioutil.NopCloser(strings.NewReader(params.ClusterManifest)))
	if err != nil {
		return capeios.SeedNodeParams{}, err
	}
	_, eims, err := machine.Parse(ioutil.NopCloser(strings.NewReader(params.MachinesManifest)))
	if err != nil {
		return capeios.SeedNodeParams{}, err
	}
	sshKey, err := ioutil.ReadFile(eic.Spec.DeprecatedSSHKeyPath)
	if err != nil {
		return capeios.SeedNodeParams{}, err
	}
	encodedKey := base64.StdEncoding.EncodeToString(sshKey)
	return augmentParamsWithPool(eic.Spec.User, encodedKey, eims, params), nil
}

func augmentParamsWithPool(user, key string, eim []*existinginfrav1.ExistingInfraMachine, params capeios.SeedNodeParams) capeios.SeedNodeParams {
	info := []capeios.MachineInfo{}
	for _, m := range eim {
		info = append(info, capeios.MachineInfo{
			SSHUser:     user,
			SSHKey:      key,
			PublicIP:    m.Spec.Public.Address,
			PublicPort:  fmt.Sprintf("%d", m.Spec.Public.Port),
			PrivateIP:   m.Spec.Private.Address,
			PrivatePort: fmt.Sprintf("%d", m.Spec.Private.Port)})
	}
	params.ConnectionInfo = info
	return params
}

//updateSecretParams creates secret resources for auth(n/z) which get added to the seed node plan
func createSecretPlan(o *capeios.OS, params capeios.SeedNodeParams) (plan.Resource, capeios.SeedNodeParams, error) {
	pemPlan, pemSecretResources, authConfigMap, authConfigManifest, err := processPemFilesIfAny(&params.ExistingInfraCluster.Spec, params.ConfigDirectory, params.Namespace, params.SealedSecretKey, params.SealedSecretCert)
	if err != nil {
		return nil, params, err
	}
	if pemSecretResources == nil {
		return nil, params, nil
	}
	info := &capeios.AuthParams{PEMSecretResources: pemSecretResources, AuthConfigMap: authConfigMap, AuthConfigManifest: authConfigManifest}
	newParams := params
	newParams.AuthInfo = info
	return pemPlan, newParams, nil
}

// processPemFilesIfAny reads the SealedSecret from the config
// directory, decrypts it using the GitHub deploy key, creates file
// resources for .pem files stored in the secret, and creates a SealedSecret resource
// for them that can be used by the machine actuator
func processPemFilesIfAny(providerSpec *existinginfrav1.ClusterSpec, configDir string, ns, privateKey, cert string) (plan.Resource, map[string]*capeios.SecretResourceSpec, *v1.ConfigMap, []byte, error) {
	if err := checkPemValues(providerSpec, privateKey, cert); err != nil {
		return nil, nil, nil, nil, err
	}
	if providerSpec.Authentication == nil && providerSpec.Authorization == nil {
		// no auth specified
		return nil, nil, nil, nil, nil
	}
	b := plan.NewBuilder()
	b.AddResource("create:pem-dir", &capeiresource.Dir{Path: object.String(capeios.PemDestDir)})
	b.AddResource("set-perms:pem-dir", &capeiresource.Run{Script: object.String(fmt.Sprintf("chmod 600 %s", capeios.PemDestDir))}, plan.DependOn("create:pem-dir"))
	rsaPrivateKey, err := getPrivateKey(privateKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var authenticationSecretFileName, authorizationSecretFileName, authenticationSecretName, authorizationSecretName string
	var authenticationSecretManifest, authorizationSecretManifest, authenticationConfig, authorizationConfig []byte
	var decrypted map[string][]byte
	secretResources := map[string]*capeios.SecretResourceSpec{}
	if providerSpec.Authentication != nil {
		authenticationSecretFileName = providerSpec.Authentication.SecretFile
		authenticationSecretManifest, decrypted, authenticationSecretName, authenticationConfig, err = processSecret(
			b, rsaPrivateKey, configDir, authenticationSecretFileName, providerSpec.Authentication.URL)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		secretResources["authentication"] = &capeios.SecretResourceSpec{
			SecretName: authenticationSecretName,
			Decrypted:  decrypted,
			Resource:   &capeiresource.KubectlApply{Namespace: object.String(ns), Manifest: authenticationSecretManifest, Filename: object.String(authenticationSecretName)}}
	}
	if providerSpec.Authorization != nil {
		authorizationSecretFileName = providerSpec.Authorization.SecretFile
		authorizationSecretManifest, decrypted, authorizationSecretName, authorizationConfig, err = processSecret(
			b, rsaPrivateKey, configDir, authorizationSecretFileName, providerSpec.Authorization.URL)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		secretResources["authorization"] = &capeios.SecretResourceSpec{
			SecretName: authorizationSecretName,
			Decrypted:  decrypted,
			Resource:   &capeiresource.KubectlApply{Namespace: object.String(ns), Manifest: authorizationSecretManifest, Filename: object.String(authorizationSecretName)}}
	}
	filePlan, err := b.Plan()
	if err != nil {
		log.Infof("Plan creation failed:\n%s\n", err)
		return nil, nil, nil, nil, err
	}
	authConfigMap, authConfigMapManifest, err := createAuthConfigMapManifest(authenticationSecretName, authorizationSecretName,
		authenticationConfig, authorizationConfig)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return &filePlan, secretResources, authConfigMap, authConfigMapManifest, nil
}

func getPrivateKey(privateKey string) (*rsa.PrivateKey, error) {
	privateKeyBytes := []byte(privateKey)
	privateKeyData, err := keyutil.ParsePrivateKeyPEM(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	rsaPrivateKey, ok := privateKeyData.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("Invalid private key")
	}
	return rsaPrivateKey, nil
}

func checkPemValues(providerSpec *existinginfrav1.ClusterSpec, privateKey, cert string) error {
	if privateKey == "" || cert == "" {
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
	capeios.StoreIfNotEmpty(data, "authentication-secret-name", authnSecretName)
	capeios.StoreIfNotEmpty(data, "authorization-secret-name", authzSecretName)
	capeios.StoreIfNotEmpty(data, "authentication-config", string(authnConfig))
	capeios.StoreIfNotEmpty(data, "authorization-config", string(authzConfig))
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

// getConfigFileContents reads a config manifest from a file in the config directory.
func getConfigFileContents(fileNameComponent ...string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(fileNameComponent...))
}
