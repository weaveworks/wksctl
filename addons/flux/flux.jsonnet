local k = import '../vendor/ksonnet/ksonnet.beta.3/k.libsonnet';
local mixin = import '../mixin.libsonnet';

local serviceAccount = k.core.v1.serviceAccount;
local clusterRole = k.rbac.v1beta1.clusterRole;
local rulesType = k.rbac.v1beta1.clusterRole.rulesType;
local clusterRoleBinding = k.rbac.v1beta1.clusterRoleBinding;
local subject = clusterRoleBinding.subjectsType;
local secret = k.core.v1.secret;
local deployment = k.apps.v1beta1.deployment;
local container = deployment.mixin.spec.template.spec.containersType;
local containerPort = container.portsType;
local volumeMount = container.volumeMountsType;
local volume = deployment.mixin.spec.template.spec.volumesType;
local service = k.core.v1.service;
local servicePort = k.core.v1.service.mixin.spec.portsType;

function(
    gitURL='git@github.com/owner/repo',
    gitBranch='master',
    namespace='flux',
    imageRepository=null,
    gitDeployKey=null,
	gitPath='.',
  )

  local config = {
    namespace: namespace,
    flux: {
      name: 'flux',
      labels: {
        name: 'flux', // Ought to match config.flux.name
      },
      replicas: 1,
      port: 3030,
      prometheus: {
        port: 3031,
      },
      keygenDir: '/var/fluxd/keygen',
      secret: 'flux-git-deploy',
      deployKey: gitDeployKey,
      git: {
        url: gitURL,
        branch: gitBranch,
        pollInterval: '30s',
		path: gitPath,
      },
      image: {
        repo: 'fluxcd/flux',
        version: '1.21.0',
      },
    },
    memcached: {
      name: 'memcached',
      labels: {
        name: 'memcached', // Ought to match config.memcached.name
      },
      replicas: 1,
      maxMemoryInMB: 64,
      port: 11211,
      image: {
        repo: 'memcached',
        version: '1.4.25',
      },
    },
    tolerations: [
    # Allow scheduling on master nodes. This is required because during
    # bootstrapping of the cluster, we may initially have just one master,
    # and would then need to deploy this controller there to set the entire
    # cluster up.
    {effect: 'NoSchedule',
      key: 'node-role.kubernetes.io/master',
      operator: 'Exists'},
    # Mark this as a critical addon:
    {key: 'CriticalAddonsOnly',
      operator: 'Exists'}
    ],

    image:: function(key)
      self[key].image.repo + ":" + self[key].image.version
  } +
  mixin.withImageRepository(imageRepository);

  local ns = k.core.v1.namespace.new(config.namespace);

  local sa = serviceAccount.new(config.flux.name) +
    serviceAccount.mixin.metadata.withNamespace(config.namespace) +
    serviceAccount.mixin.metadata.withLabels(config.flux.labels);

  local cr = clusterRole.new() +
    clusterRole.mixin.metadata.withName(config.flux.name) +
    clusterRole.mixin.metadata.withNamespace(config.namespace) +
    clusterRole.mixin.metadata.withLabels(config.flux.labels) +
    clusterRole.withRules([
      rulesType.withApiGroups(['*']).withResources(['*']).withVerbs(['*']),
      rulesType.withNonResourceUrls(['*']).withVerbs(['*'])
    ]);

  local crb = clusterRoleBinding.new() +
    clusterRoleBinding.mixin.metadata.withName(config.flux.name) +
    clusterRoleBinding.mixin.metadata.withNamespace(config.namespace) +
    clusterRoleBinding.mixin.metadata.withLabels(config.flux.labels) +
    clusterRoleBinding.mixin.roleRef.mixinInstance({
        kind: 'ClusterRole',
        name: config.flux.name,
        apiGroup: 'rbac.authorization.k8s.io',
    }) +
    clusterRoleBinding.withSubjects([
      subject.new() +
        {kind: 'ServiceAccount'} +
        subject.withName(config.flux.name) +
        subject.withNamespace(config.namespace),
    ]);

  local memcached = container.new(config.memcached.name, config.image('memcached'))
    .withImagePullPolicy('IfNotPresent')
    .withArgs([
      '-m %s' % [config.memcached.maxMemoryInMB],
      '-p %s' % [config.memcached.port],
    ])
    .withPorts([
      containerPort.newNamed('clients', config.memcached.port),
    ]);

  local memcachedDeployment = deployment.new(config.memcached.name, config.memcached.replicas, [memcached], config.memcached.labels) +
    deployment.mixin.metadata.withNamespace(config.namespace) +
    deployment.mixin.spec.selector.withMatchLabels(config.memcached.labels);

  local memcachedService = service.new(
      config.memcached.name,
      memcachedDeployment.spec.template.metadata.labels,
      servicePort.newNamed(config.memcached.name, config.memcached.port, config.memcached.port)
    ) +
    service.mixin.metadata.withNamespace(config.namespace) +
    service.mixin.spec.withClusterIp('None');

  local fluxGitDeploySecret = secret.new(config.flux.secret, {}, 'Opaque') +
    secret.mixin.metadata.withNamespace(config.namespace);

 local flux = container.new(config.flux.name, config.image('flux'))
    .withImagePullPolicy('IfNotPresent')
    .withArgs([
      '--ssh-keygen-dir=%s' % [config.flux.keygenDir],
      '--git-url=%s' % [config.flux.git.url],
      '--git-branch=%s' % [config.flux.git.branch],
      '--git-poll-interval=%s' % [config.flux.git.pollInterval],
      '--git-path="%s"' % [config.flux.git.path],
      '--memcached-hostname=%s.%s.svc.cluster.local' % [config.memcached.name, config.namespace],
      '--memcached-service=%s' % [config.memcached.name],
      '--listen-metrics=:%s' % [config.flux.prometheus.port],
      '--sync-garbage-collection',
    ])
    .withPorts([
      containerPort.new(config.flux.port),
    ])
    .withVolumeMounts([
      volumeMount.new(
        'git-key',
        '/etc/fluxd/ssh', // to match location given in image's /etc/ssh/config
        readOnly=true,
      ),
      volumeMount.new(
        'git-keygen',
        config.flux.keygenDir, // to match location given in image's /etc/ssh/config
        readOnly=false,
      ),
    ]);

  local fluxDeployment = deployment.new(config.flux.name, config.flux.replicas, [flux], config.flux.labels) +
    deployment.mixin.metadata.withNamespace(config.namespace) +
    deployment.mixin.spec.selector.withMatchLabels(config.flux.labels) +
    deployment.mixin.spec.strategy.withType('Recreate') +
    deployment.mixin.spec.template.metadata
      .withAnnotations({
        'prometheus.io.port': std.toString(config.flux.prometheus.port)
      })
      .withLabels(config.flux.labels) +
    deployment.mixin.spec.template.spec.withServiceAccount(config.flux.name) +
    deployment.mixin.spec.template.spec.withVolumes([
      volume.fromSecret('git-key', config.flux.secret).withDefaultMode(std.parseOctal('0400')), // when mounted read-only, we won't be able to chmod
      volume.fromEmptyDir('git-keygen', {medium: 'Memory'}),
    ]);



  k.core.v1.list.new([
    ns,
    sa,
    cr,
    crb,
    memcachedDeployment + deployment.mixin.spec.template.spec.withTolerations(config.tolerations) ,
    memcachedService,
    if config.flux.deployKey != null && config.flux.deployKey != "" then fluxGitDeploySecret + secret.withData({'identity':config.flux.deployKey}) else fluxGitDeploySecret,
    fluxDeployment + deployment.mixin.spec.template.spec.withTolerations(config.tolerations)
  ])
