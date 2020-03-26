package envcfg

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
)

const (
	cmdOsRel             = "cat /etc/os-release"
	cmdEnv               = "cat /proc/1/environ"
	cmdSELinuxFound      = `command -v -- "selinuxenabled" >/dev/null 2>&1`
	cmdSELinuxEnabled    = `selinuxenabled`
	cmdMachineID         = "cat /etc/machine-id 2>/dev/null || cat /var/lib/dbus/machine-id 2>/dev/null"
	cmdUUID              = "cat /sys/class/dmi/id/product_uuid 2>/dev/null || cat /etc/machine-id 2>/dev/null"
	cmdSELinuxPermissive = "sestatus | grep 'Current mode' | grep permissive"
	cmdSELinuxEnforcing  = "sestatus | grep 'Current mode' | grep enforcing"

	relUbuntu    = "ID=ubuntu\nVERSION_ID=\"18.04\""
	relCentos    = "ID=centos\nVERSION_ID=7\n"
	envContainer = "container=docker\n"
)

type runnerResult struct {
	out string
	err error
}

type fakeRunner struct {
	value map[string]runnerResult
}

func (f *fakeRunner) RunCommand(cmd string, _ io.Reader) (stdouterr string, err error) {
	val, ok := f.value[cmd]
	if !ok {
		panic(fmt.Sprintf("fakeRunner: asked to run command without defined result: %q", cmd))
	}
	return val.out, val.err
}

// TestGetEnvSpecificConfig simulates various system states (by means of faked results of individual commands), computes EnvSpecificConfig based on that and checks that results match expectations.
func TestGetEnvSpecificConfig(t *testing.T) {
	for _, tt := range []struct {
		name string

		// Inputs to EnvSpecificConfig
		pkgType       resource.PkgType
		cloudProvider string
		runner        plan.Runner

		// Expected results
		wantUseIPTables, wantSetSELinuxPermissive, wantDisableSwap, wantLockYUMPkgs bool
		wantIgnorePreflightErrors                                                   []string
		wantNamespace, wantHostnameOverride                                         string

		// Expected error
		wantError bool
	}{
		{
			name:    "container, ubuntu, selinux enabled",
			pkgType: resource.PkgTypeDeb,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {out: envContainer},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {},
					cmdSELinuxEnforcing:  {},
				},
			},

			wantUseIPTables:           false,
			wantSetSELinuxPermissive:  false,
			wantDisableSwap:           false,
			wantLockYUMPkgs:           false,
			wantIgnorePreflightErrors: []string{FC_bridge_nf_call_iptables, Swap, SystemVerification},
			wantNamespace:             "foo",
		},
		{
			name:    "container, ubuntu, selinux not found",
			pkgType: resource.PkgTypeDeb,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {out: envContainer},
					cmdSELinuxFound:      {err: &plan.RunError{ExitCode: 1}},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {err: &plan.RunError{ExitCode: 1}},
					cmdSELinuxEnforcing:  {err: &plan.RunError{ExitCode: 1}},
				},
			},

			wantUseIPTables:           false,
			wantSetSELinuxPermissive:  false,
			wantDisableSwap:           false,
			wantLockYUMPkgs:           false,
			wantIgnorePreflightErrors: []string{FC_bridge_nf_call_iptables, Swap, SystemVerification},
			wantNamespace:             "foo",
		},
		{
			name:    "container, centos, selinux found and enabled",
			pkgType: resource.PkgTypeRPM,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relCentos},
					cmdEnv:               {out: envContainer},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {}, //, err: &plan.RunError{ExitCode: 1}},
					cmdSELinuxEnforcing:  {},
				},
			},

			wantUseIPTables:           false,
			wantSetSELinuxPermissive:  false,
			wantDisableSwap:           false,
			wantLockYUMPkgs:           true,
			wantIgnorePreflightErrors: []string{FC_bridge_nf_call_iptables, Swap, SystemVerification},
			wantNamespace:             "foo",
		},
		{
			name:    "vm, centos, selinux found and enabled",
			pkgType: resource.PkgTypeRPM,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relCentos},
					cmdEnv:               {},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {err: &plan.RunError{ExitCode: 1}},
					cmdSELinuxEnforcing:  {},
				},
			},

			wantUseIPTables:          true,
			wantSetSELinuxPermissive: true,
			wantDisableSwap:          true,
			wantLockYUMPkgs:          true,
			wantNamespace:            "foo",
		},
		{
			name:    "vm, centos, selinux not found",
			pkgType: resource.PkgTypeRPM,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relCentos},
					cmdEnv:               {},
					cmdSELinuxFound:      {err: &plan.RunError{ExitCode: 1}},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {err: &plan.RunError{ExitCode: 1}},
					cmdSELinuxEnforcing:  {err: &plan.RunError{ExitCode: 1}},
				},
			},

			wantUseIPTables:          true,
			wantSetSELinuxPermissive: false,
			wantDisableSwap:          true,
			wantLockYUMPkgs:          true,
			wantNamespace:            "foo",
		},
		{
			name:    "vm, ubuntu, selinux not found",
			pkgType: resource.PkgTypeDeb,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {},
					cmdSELinuxFound:      {err: &plan.RunError{ExitCode: 1}},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {},
					cmdSELinuxEnforcing:  {},
				},
			},

			wantUseIPTables:          true,
			wantSetSELinuxPermissive: false,
			wantDisableSwap:          true,
			wantLockYUMPkgs:          false,
			wantNamespace:            "foo",
		},
		{
			name:    "vm, ubuntu, selinux not found, exitcode 127",
			pkgType: resource.PkgTypeDeb,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {},
					cmdSELinuxFound:      {err: &plan.RunError{ExitCode: 127}},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {},
					cmdSELinuxEnforcing:  {},
				},
			},

			wantUseIPTables:          true,
			wantSetSELinuxPermissive: false,
			wantDisableSwap:          true,
			wantLockYUMPkgs:          false,
			wantNamespace:            "foo",
		},
		{
			name:    "vm, ubuntu, selinux found and enabled",
			pkgType: resource.PkgTypeDeb,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {err: &plan.RunError{ExitCode: 1}},
					cmdSELinuxEnforcing:  {},
				},
			},

			wantUseIPTables:          true,
			wantSetSELinuxPermissive: true,
			wantDisableSwap:          true,
			wantLockYUMPkgs:          false,
			wantNamespace:            "foo",
		},
		{
			name:    "vm, ubuntu, selinux found but not enabled",
			pkgType: resource.PkgTypeDeb,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {err: &plan.RunError{ExitCode: 1}},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {err: &plan.RunError{ExitCode: 1}},
					cmdSELinuxEnforcing:  {err: &plan.RunError{ExitCode: 1}},
				},
			},

			wantUseIPTables:          true,
			wantSetSELinuxPermissive: false,
			wantDisableSwap:          true,
			wantLockYUMPkgs:          false,
			wantNamespace:            "foo",
		},
		{
			name:    "os-release error",
			pkgType: resource.PkgTypeRPM,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu, err: errors.New("kaboom")},
					cmdEnv:               {},
					cmdSELinuxFound:      {},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {},
					cmdSELinuxEnforcing:  {},
				},
			},
			wantError: true,
		},
		{
			name:    "environ error",
			pkgType: resource.PkgTypeRPM,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {err: errors.New("kaboom")},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {},
					cmdSELinuxEnforcing:  {},
				},
			},
			wantError: true,
		},
		{
			name:    "selinuxenabled error",
			pkgType: resource.PkgTypeRPM,
			runner: &fakeRunner{
				value: map[string]runnerResult{
					cmdOsRel:             {out: relUbuntu},
					cmdEnv:               {},
					cmdSELinuxFound:      {},
					cmdSELinuxEnabled:    {err: &plan.RunError{ExitCode: 127}},
					cmdMachineID:         {out: "01234567"},
					cmdUUID:              {out: "01234567"},
					cmdSELinuxPermissive: {},
					cmdSELinuxEnforcing:  {},
				},
			},
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// act
			cfg, err := GetEnvSpecificConfig(tt.pkgType, tt.wantNamespace, tt.cloudProvider, tt.runner)

			// assert
			if (err != nil) != tt.wantError {
				t.Fatalf("got error: %v, want error? %t", err, tt.wantError)
			}

			if !tt.wantError {
				assert.Equal(t, tt.wantUseIPTables, cfg.UseIPTables)
				assert.Equal(t, tt.wantSetSELinuxPermissive, cfg.SetSELinuxPermissive)
				assert.Equal(t, tt.wantDisableSwap, cfg.DisableSwap)
				assert.Equal(t, tt.wantLockYUMPkgs, cfg.LockYUMPkgs)
				assert.ElementsMatch(t, tt.wantIgnorePreflightErrors, cfg.IgnorePreflightErrors)
				assert.Equal(t, tt.wantNamespace, cfg.Namespace)
				assert.Equal(t, tt.wantHostnameOverride, cfg.HostnameOverride)
			}
		})
	}
}
