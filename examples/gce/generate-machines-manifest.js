import * as std from '@jkcfg/std';
import * as param from '@jkcfg/std/param';

const input = param.String("instances", "instances.json");
const user = param.String("user");
const numMasters = 1;
const numWorkers = 2;

function required(name, v) {
  if (v === undefined) {
    throw new Error(`'${name}' parameter must be provided`);
  }
}

// XXX: jk should support that use case, see:
//   https://github.com/jkcfg/jk/issues/153
required("user", user);

// vm is the name of the i th vm.
const vm = i => `${user}-wks-${i}`;

// getInstance returns the instance named name from a list of instances
function getInstance(instances, name) {
  for (let i = 0; i < instances.length; i++) {
    if (instances[i].name == name) {
      return instances[i];
    }
  }
  throw new Error(`instance '${name}' not found`);
}

// Machine returns a WKS machine description from a `gcloud compute instances
// list` instance JSON.
const Machine = (instance, role) => ({
  apiVersion: 'cluster.x-k8s.io/v1alpha3',
  kind: 'Machine',
  metadata: {
    name: `${role}-`+instance.networkInterfaces[0].accessConfigs[0].natIP,
    labels: {
      set: role,
    },
  },
  spec: {
    clusterName: 'example-gce',
    infrastructureRef: {
      apiVersion: 'cluster.weave.works/v1alpha3',
      kind: 'ExistingInfraMachine',
      name: `${role}-`+instance.networkInterfaces[0].accessConfigs[0].natIP,
    }
  }
});

const ExistingInfraMachine = (instance, role) => ({
  apiVersion: 'cluster.weave.works/v1alpha3',
  kind: 'ExistingInfraMachine',
  metadata: {
    name: `${role}-`+instance.networkInterfaces[0].accessConfigs[0].natIP,
    labels: {
      set: role,
    },
  },
  spec: {
    public: {
      address: instance.networkInterfaces[0].accessConfigs[0].natIP,
      port: 22,
    },
    private: {
      address: instance.networkInterfaces[0].networkIP,
      port: 22,
    }
  }
});

std.read(input).then(instances => {
  let machines = [];

  for (let i = 1; i < numMasters + 1; i++) {
    machines.push(Machine(getInstance(instances, vm(i)), 'master'));
    machines.push(ExistingInfraMachine(getInstance(instances, vm(i)), 'master'));
  }

  for (let i = numMasters + 1; i < numMasters + numWorkers + 1; i++) {
    machines.push(Machine(getInstance(instances, vm(i)), 'worker'));
    machines.push(ExistingInfraMachine(getInstance(instances, vm(i)), 'worker'));
  }

  std.write(machines, "machines.yaml", {format: std.Format.YAMLStream});
});


