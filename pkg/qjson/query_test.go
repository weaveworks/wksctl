package qjson_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/qjson"
)

func TestCollectStringsOnScalarShouldReturnEmptyResultsSet(t *testing.T) {
	strings, err := qjson.CollectStrings("", []byte(`"foobar"`))
	assert.NoError(t, err)
	assert.Equal(t, []string{}, strings)
}
func TestCollectStringsOnObjectShouldReturnMatchingValue(t *testing.T) {
	strings, err := qjson.CollectStrings("a.c", []byte(`{"a": {"b": "B", "c": "C"}}`))
	assert.NoError(t, err)
	assert.Equal(t, []string{"C"}, strings)
}

func TestCollectStringsOnArrayOfObjectsShouldReturnMatchingValues(t *testing.T) {
	strings, err := qjson.CollectStrings("a.c", []byte(`[{"a": {"b": "B", "c": "C1"}}, {"a": {"b": "B", "c": "C2"}}]`))
	assert.NoError(t, err)
	assert.Equal(t, []string{"C1", "C2"}, strings)
}

const SampleJSON = `{
	"apiVersion": "v1",
	"items": [
	   {
		  "apiVersion": "v1",
		  "data": {
			 "kubernetes.keytab": ""
		  },
		  "kind": "Secret",
		  "metadata": {
			 "name": "keytab-secret",
			 "namespace": "kube-system"
		  },
		  "type": "Opaque"
	   },
	   {
		  "apiVersion": "apps/v1beta2",
		  "kind": "DaemonSet",
		  "metadata": {
			 "labels": {
				"name": "kube-kerberos"
			 },
			 "name": "kube-kerberos",
			 "namespace": "kube-system"
		  },
		  "spec": {
			 "selector": {
				"matchLabels": {
				   "name": "kube-kerberos"
				}
			 },
			 "template": {
				"metadata": {
				   "annotations": {
					  "scheduler.alpha.kubernetes.io/critical-pod": ""
				   },
				   "labels": {
					  "name": "kube-kerberos"
				   }
				},
				"spec": {
				   "containers": [
					  {
						 "args": [
							"--keytab=/etc/keytab-volume/kubernetes.keytab",
							"--service-account=kubernetes/kubernetes"
						 ],
						 "image": "quay.io/wksctl/k8s-krb5-server:master-8b61a17",
						 "name": "kube-kerberos",
						 "volumeMounts": [
							{
							   "mountPath": "/etc/keytab-volume",
							   "name": "keytab-volume",
							   "readOnly": false
							}
						 ]
					  }
				   ],
				   "hostNetwork": true,
				   "nodeSelector": {
					  "node-role.kubernetes.io/master": ""
				   },
				   "restartPolicy": "Always",
				   "tolerations": [
					  {
						 "effect": "NoSchedule",
						 "key": "node-role.kubernetes.io/master"
					  }
				   ],
				   "volumes": [
					  {
						 "name": "keytab-volume",
						 "secret": {
							"secretName": "keytab-secret"
						 }
					  }
				   ]
				}
			 }
		  }
	   }
	],
	"kind": "List"
 }`

const plainObjectJSON = `{
	"apiVersion": "apps/v1beta2",
	"kind": "Deployment",
	"metadata": {
	   "labels": {
		  "k8s-app": "prometheus-operator"
	   },
	   "name": "prometheus-operator",
	   "namespace": "monitoring"
	},
	"spec": {
	   "replicas": 1,
	   "selector": {
		  "matchLabels": {
			 "k8s-app": "prometheus-operator"
		  }
	   },
	   "template": {
		  "metadata": {
			 "labels": {
				"k8s-app": "prometheus-operator"
			 }
		  },
		  "spec": {
			 "containers": [
				{
				   "args": [
					  "--kubelet-service=kube-system/kubelet",
					  "--logtostderr=true",
					  "--config-reloader-image=quay.io/coreos/configmap-reload:v0.0.1",
					  "--prometheus-config-reloader=quay.io/coreos/prometheus-config-reloader:v0.25.0"
				   ],
				   "image": "quay.io/coreos/prometheus-operator:v0.25.0",
				   "name": "prometheus-operator",
				   "ports": [
					  {
						 "containerPort": 8080,
						 "name": "http"
					  }
				   ],
				   "resources": {
					  "limits": {
						 "cpu": "200m",
						 "memory": "200Mi"
					  },
					  "requests": {
						 "cpu": "100m",
						 "memory": "100Mi"
					  }
				   },
				   "securityContext": {
					  "allowPrivilegeEscalation": false,
					  "readOnlyRootFilesystem": true
				   }
				}
			 ],
			 "nodeSelector": {
				"beta.kubernetes.io/os": "linux"
			 },
			 "securityContext": {
				"runAsNonRoot": true,
				"runAsUser": 65534
			 },
			 "serviceAccountName": "prometheus-operator"
		  }
	   }
	}
 }`

func TestCollectStringsForImagesInKubernetesManifest(t *testing.T) {
	strings, err := qjson.CollectStrings("spec.containers.#.image", []byte(SampleJSON))
	assert.NoError(t, err)
	assert.Equal(t, []string{"quay.io/wksctl/k8s-krb5-server:master-8b61a17"}, strings)
}

func TestCollectStringsForImagesInPlainKubernetesManifest(t *testing.T) {
	strings, err := qjson.CollectStrings("spec.containers.#.image", []byte(plainObjectJSON))
	assert.NoError(t, err)
	assert.Equal(t, []string{"quay.io/coreos/prometheus-operator:v0.25.0"}, strings)
}

func TestCollectStringsForArrayValuesInKubernetesManifest(t *testing.T) {
	strings, err := qjson.CollectStrings("spec.containers.#.args.#", []byte(SampleJSON))
	assert.NoError(t, err)
	assert.Equal(t, []string{
		"--keytab=/etc/keytab-volume/kubernetes.keytab",
		"--service-account=kubernetes/kubernetes",
	}, strings)
}

func TestCollectStringsForVolumeNamesInKubernetesManifest(t *testing.T) {
	strings, err := qjson.CollectStrings("spec.containers.#.volumeMounts.#.name", []byte(SampleJSON))
	assert.NoError(t, err)
	assert.Equal(t, []string{"keytab-volume"}, strings)
}
