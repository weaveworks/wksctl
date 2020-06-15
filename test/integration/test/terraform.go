package test

import (
	"encoding/json"
	"io"
	"os"
)

const (
	keyPublicIPs  = "public_ips"
	keyPrivateIPs = "private_ips"
)

type terraformVariable struct {
	Sensitive bool
	Type      string
	Value     interface{}
}

func (v *terraformVariable) asString() string {
	return v.Value.(string)
}

func (v *terraformVariable) asStringArray() []string {
	a := v.Value.([]interface{})
	r := make([]string, len(a))
	for i, v := range a {
		r[i] = v.(string)
	}
	return r
}

type terraformOutput struct {
	data map[string]*terraformVariable
}

func newTerraformOutput(r io.Reader) (*terraformOutput, error) {
	output := &terraformOutput{
		data: make(map[string]*terraformVariable),
	}
	decoder := json.NewDecoder(r)

	if err := decoder.Decode(&output.data); err != nil {
		return nil, err
	}

	return output, nil
}

func newTerraformOutputFromFile(path string) (*terraformOutput, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return newTerraformOutput(r)
}

func (o *terraformOutput) numMachines() int {
	return len(o.stringArrayVar(keyPublicIPs))
}

func (o *terraformOutput) stringVar(name string) string {
	return o.data[name].asString()
}

func (o *terraformOutput) stringArrayVar(name string) []string {
	return o.data[name].asStringArray()
}
