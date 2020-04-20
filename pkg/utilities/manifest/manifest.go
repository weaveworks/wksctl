package manifest

import (
	encjson "encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/pkg/errors"
	bmv1alpha3 "github.com/weaveworks/wksctl/pkg/baremetal/v1alpha3"
	"gopkg.in/oleiade/reflections.v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/yaml"
)

var mutex *sync.Mutex = &sync.Mutex{}

// cluster-api types
const (
	GroupName        = "sigs.k8s.io"
	GroupVersion     = "v1alpha3"
	DefaultNamespace = `weavek8sops`
)

var DefaultAddonNamespaces = map[string]string{"weave-net": "kube-system"}

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: GroupVersion}
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&clusterv1alpha3.Cluster{},
		&clusterv1alpha3.ClusterList{},
		&clusterv1alpha3.Machine{},
		&clusterv1alpha3.MachineList{},
	)
	scheme.AddKnownTypes(bmv1alpha3.SchemeGroupVersion,
		&bmv1alpha3.BareMetalCluster{},
		&bmv1alpha3.BareMetalMachine{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

//WithNamespace takes in a file or string kubernetes manifest and updates any resources to
//use the namespace specified.
//Returns the updated manifest or an error if there was a problem updating the manifest.
func WithNamespace(fileOrString, namespace string) (string, error) {
	mutex.Lock()
	clusterv1alpha3.AddToScheme(scheme.Scheme)
	mutex.Unlock()

	content, err := Content(fileOrString)
	if err != nil {
		return "", err
	}
	if namespace == "" {
		return string(content), nil
	}

	var (
		entries []string = strings.Split(string(content), "---\n")
		updates []string
	)

	for _, entry := range entries {
		if entry == "" {
			continue
		}
		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, _, err := decode([]byte(entry), nil, nil)
		if err != nil {
			return "", errors.Wrap(err, "Unable to deserialize entry")
		}
		err = updateNamespace(&obj, namespace)
		if err != nil {
			return "", errors.Wrap(err, "unable to update namespace")
		}
		updated, err := yaml.Marshal(obj)
		if err != nil {
			return "", errors.Wrap(err, "unable to marshal object")
		}
		updates = append(updates, string(updated))
	}
	return strings.Join(updates, "---\n"), nil
}

func updateNamespace(obj *runtime.Object, namespace string) error {
	if err := setNamespace(obj, namespace); err != nil {
		return errors.Wrap(err, "Unable to set namespace")
	}
	if err := setSubjectNamespaceRefs(obj, namespace); err != nil {
		return errors.Wrap(err, "Unable to set Subject resource namespace ref")
	}
	if meta.IsListType(*obj) {
		items, err := meta.ExtractList(*obj)
		if err != nil {
			return errors.Wrap(err, "Unable to extract items from List resource!")
		}

		s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
		for i, item := range items {
			hasRawField, err := reflections.HasField(item, "Raw")
			if err != nil {
				return errors.Wrap(err, "Unable to determine if list item has Raw field")
			}
			if hasRawField {
				r, err := reflections.GetField(item, "Raw")
				if err != nil {
					return errors.Wrap(err, "Unable to get Raw field to decode")
				}
				raw := r.([]byte)
				o, _, err := s.Decode(raw, nil, nil)
				if err != nil {
					return errors.Wrap(err, "Unable to decode item's raw field")
				}
				updateNamespace(&o, namespace)
				items[i] = o
			} else {
				updateNamespace(&item, namespace)
				items[i] = item
			}
		}
		err = meta.SetList(*obj, items)
		if err != nil {
			errors.Wrap(err, "Unable to set items on List resource")
		}
	}
	return nil
}

func setNamespace(obj *runtime.Object, namespace string) error {
	k, err := reflections.GetField(*obj, "Kind")
	if err != nil {
		return errors.Wrap(err, "Unable to get object kind field")
	}
	kind := k.(string)
	hasField, err := reflections.HasField(*obj, "ObjectMeta")
	if err != nil {
		return errors.Wrap(err, "unable to determine if object has metadata")
	}
	if hasField {
		v := reflect.ValueOf(*obj).Elem().FieldByName("ObjectMeta")
		om := v.Addr().Interface().(*metav1.ObjectMeta)
		if kind == "Namespace" {
			om.SetName(namespace)
		} else {
			om.SetNamespace(namespace)
		}
		if err := reflections.SetField(*obj, "ObjectMeta", *om); err != nil {
			return errors.Wrap(err, "unable to update metadata on object")
		}
	}
	return nil
}

func setSubjectNamespaceRefs(obj *runtime.Object, namespace string) error {
	hasSubjects, err := reflections.HasField(*obj, "Subjects")
	if err != nil {
		return errors.Wrap(err, "Unable to determine if resource has Subject resources")
	}
	if !hasSubjects {
		return nil
	}
	subjects, err := reflections.GetField(*obj, "Subjects")
	if err != nil {
		return err
	}
	switch reflect.TypeOf(subjects).Kind() {
	case reflect.Slice:
		subs := reflect.ValueOf(subjects)
		for i := 0; i < subs.Len(); i++ {
			sub := subs.Index(i)
			subptr := sub.Addr().Interface()
			hasNamespace, err := reflections.HasField(subptr, "Namespace")
			if err != nil {
				return errors.Wrap(err, "Unable to determine with Subject has a namespace field")
			}
			if !hasNamespace {
				continue
			}
			if err = reflections.SetField(subptr, "Namespace", namespace); err != nil {
				return err
			}
			subs.Index(i).Set(sub)
		}
		if err = reflections.SetField(*obj, "Subjects", subs.Interface()); err != nil {
			return errors.Wrap(err, "Unable to set namespace references")
		}
	default:
		return errors.New("Subjects should be a slice but wasn't")
	}
	return nil
}

//Content returns a byte slice representing the yaml manifest retrieved from a passed in file or string
func Content(fileOrString string) ([]byte, error) {
	if !isFile(fileOrString) {
		return convertJSONToYAMLIfNecessary(fileOrString)
	}
	file, err := os.Open(fileOrString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open manifest")
	}

	defer file.Close()

	content, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest")
	}
	return convertJSONToYAMLIfNecessary(string(content))
}

func convertJSONToYAMLIfNecessary(yamlOrJson string) ([]byte, error) {
	if isJSON(yamlOrJson) {
		yaml, err := jsonToYaml(yamlOrJson)
		if err != nil {
			return nil, err
		}
		return yaml, nil
	}
	return []byte(yamlOrJson), nil
}

func isJSON(s string) bool {
	var js string
	return encjson.Unmarshal([]byte(s), &js) == nil
}

func jsonToYaml(content string) ([]byte, error) {
	cbytes := []byte(content)
	bytes, err := yaml.JSONToYAML(cbytes)
	if err != nil {
		return nil, err
	}
	if len(bytes) == 0 {
		return nil, errors.New("Could not convert json to yaml")
	}
	return bytes, nil
}

func isFile(fileOrString string) bool {
	_, err := os.Stat(fileOrString)
	if err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), "file name too long")) {
		return false
	}
	return true
}
