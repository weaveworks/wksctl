package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
)

func makeRPMState(name, version, release string) plan.State {
	return map[string]interface{}{
		"name":    name,
		"version": version,
		"release": release,
	}
}

func TestRPMStateDifferent(t *testing.T) {
	makeNoVersion := &RPM{
		Name: "make",
	}
	makeVersion := &RPM{
		Name:    "make",
		Version: "3.82",
	}
	makeRelease := &RPM{
		Name:    "make",
		Version: "3.82",
		Release: "23.el7",
	}

	tests := []struct {
		p        *RPM
		current  plan.State
		expected bool
	}{
		{makeNoVersion, plan.EmptyState, true},
		{makeVersion, plan.EmptyState, true},
		{makeVersion, plan.EmptyState, true},
		{makeRelease, plan.EmptyState, true},

		// make already installed with a compatible (version, release)
		{makeNoVersion, makeRPMState("make", "3.82", "23.el7"), false},
		{makeVersion, makeRPMState("make", "3.82", "23.el7"), false},
		{makeRelease, makeRPMState("make", "3.82", "23.el7"), false},

		// make already installed but with an incompatible version or release.
		{makeVersion, makeRPMState("make", "3.83", "01.el7"), true},
		{makeRelease, makeRPMState("make", "3.82", "24.el7"), true},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.p.stateDifferent(test.current))
	}
}

func TestRevisionComparison(t *testing.T) {
	makeNoVersion := &RPM{
		Name: "make",
	}
	makeCurrentVersion := &RPM{
		Name:    "make",
		Version: "3.82",
	}
	makeRelease := &RPM{
		Name:    "make",
		Version: "3.82",
		Release: "23.el7",
	}
	makeNewVersion := &RPM{
		Name:    "make",
		Version: "3.83",
	}
	makeOldVersion := &RPM{
		Name:    "make",
		Version: "3.81",
	}
	tests := []struct {
		p1       *RPM
		p2       *RPM
		expected bool
	}{
		{makeNoVersion, makeOldVersion, true},
		{makeNoVersion, makeCurrentVersion, true},
		{makeNoVersion, makeRelease, true},
		{makeNoVersion, makeNewVersion, true},

		{makeOldVersion, makeNoVersion, false},
		{makeCurrentVersion, makeNoVersion, false},
		{makeRelease, makeNoVersion, false},
		{makeNewVersion, makeNoVersion, false},

		{makeOldVersion, makeCurrentVersion, true},
		{makeOldVersion, makeRelease, true},
		{makeOldVersion, makeNewVersion, true},

		{makeOldVersion, makeOldVersion, false},
		{makeCurrentVersion, makeOldVersion, false},
		{makeRelease, makeOldVersion, false},
		{makeNewVersion, makeOldVersion, false},

		{makeCurrentVersion, makeRelease, true},
		{makeCurrentVersion, makeNewVersion, true},

		{makeCurrentVersion, makeCurrentVersion, false},
		{makeRelease, makeCurrentVersion, false},
		{makeNewVersion, makeCurrentVersion, false},

		{makeRelease, makeNewVersion, true},

		{makeRelease, makeRelease, false},
		{makeNewVersion, makeRelease, false},

		{makeNewVersion, makeNewVersion, false},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, lowerRevisionThan(test.p1.State(), test.p2.State()))
	}
}
