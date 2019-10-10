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
  apiVersion: 'cluster.k8s.io/v1alpha1',
  kind: 'Machine',
  metadata: {
    generateName: `${role}-`,
    labels: {
      set: role,
    },
  },
  spec: {
    providerSpec: {
      value: {
        apiVersion: 'baremetalproviderspec/v1alpha1',
        kind: 'BareMetalMachineProviderSpec',
        public: {
          address: instance.networkInterfaces[0].accessConfigs[0].natIP,
          port: 22,
        },
        private: {
          address: instance.networkInterfaces[0].networkIP,
          port: 22,
        }
      }
    }
  }
});

// List is a Kubernetes list.
const List = items => ({
  apiVersion: "v1",
  kind: "List",
  items
});

std.read(input).then(instances => {
  let machines = [];

  for (let i = 1; i < numMasters + 1; i++) {
    machines.push(Machine(getInstance(instances, vm(i)), 'master'));
  }

  for (let i = numMasters + 1; i < numMasters + numWorkers + 1; i++) {
    machines.push(Machine(getInstance(instances, vm(i)), 'worker'));
  }

  std.write(List(machines), "machines.yaml");
});

