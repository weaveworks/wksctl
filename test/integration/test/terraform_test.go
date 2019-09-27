package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const sampleTerraformOutput = `{
    "ansible_inventory": {
        "sensitive": false,
        "type": "string",
        "value": "[all]\n35.238.98.255 private_ip=10.128.0.5\n35.238.50.88 private_ip=10.128.0.4"
    },
    "hostnames": {
        "sensitive": false,
        "type": "string",
        "value": "test-131-0-0.us-central1-a.wks\ntest-131-0-1.us-central1-a.wks"
    },
    "image": {
        "sensitive": false,
        "type": "string",
        "value": "wks-centos7-docker1706"
    },
    "instances_names": {
        "sensitive": false,
        "type": "list",
        "value": [
            "test-131-0-0",
            "test-131-0-1"
        ]
    },
    "private_etc_hosts": {
        "sensitive": false,
        "type": "string",
        "value": "10.128.0.5 test-131-0-0.us-central1-a.wks\n10.128.0.4 test-131-0-1.us-central1-a.wks"
    },
    "private_key_path": {
        "sensitive": false,
        "type": "string",
        "value": "/root/.ssh/wksctl_cit_id_rsa"
    },
    "public_etc_hosts": {
        "sensitive": false,
        "type": "string",
        "value": "35.238.98.255 test-131-0-0.us-central1-a.wks\n35.238.50.88 test-131-0-1.us-central1-a.wks"
    },
    "public_ips": {
        "sensitive": false,
        "type": "list",
        "value": [
            "35.238.98.255",
            "35.238.50.88"
        ]
    },
    "private_ips": {
        "sensitive": false,
        "type": "list",
        "value": [
            "10.128.0.5",
            "10.128.0.4"
        ]
    },
    "username": {
        "sensitive": false,
        "type": "string",
        "value": "wksctl-cit"
    },
    "zone": {
        "sensitive": false,
        "type": "string",
        "value": "us-central1-a"
    }
}
`

func TestNewTerraformOutput(t *testing.T) {
	r := strings.NewReader(sampleTerraformOutput)
	output, err := newTerraformOutput(r)
	assert.NoError(t, err)
	assert.Equal(t, "wksctl-cit", output.stringVar("username"))
	assert.Equal(t, []string{"35.238.98.255", "35.238.50.88"}, output.stringArrayVar("public_ips"))

}
