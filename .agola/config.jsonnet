local go_runtime(version, arch) = {
  type: 'pod',
  arch: arch,
  containers: [
    {
      image: 'golang:' + version + '-buster',
    },
  ],
};

local ci_runtime(pgversion, arch) = {
  type: 'pod',
  arch: arch,
  containers: [
    {
      image: 'sorintlab/stolon-ci-image:v0.2.0-pg' + pgversion,
      volumes: [
        {
          path: '/stolontemp',
          tmpfs: {},
        },
      ],
    },
  ],
};

local dind_runtime(arch) = {
  type: 'pod',
  arch: arch,
  containers: [
    {
      image: 'docker:stable-dind',
      privileged: true,
      entrypoint: 'dockerd --bip 172.18.0.1/16',
    },
  ],
};

local task_build_go(version, arch) = {
  name: 'build go ' + version + ' ' + arch,
  runtime: go_runtime(version, arch),
  environment: {
    GO111MODULE: 'on',
  },
  steps: [
    { type: 'clone' },
    { type: 'restore_cache', keys: ['cache-sum-{{ md5sum "go.sum" }}', 'cache-date-'], dest_dir: '/go/pkg/mod/cache' },
    { type: 'run', command: 'make' },
    { type: 'run', command: 'make test' },
    { type: 'run', name: 'build integration tests binary', command: 'go test -c ./tests/integration/ -o bin/integration-tests' },
    { type: 'save_cache', key: 'cache-sum-{{ md5sum "go.sum" }}', contents: [{ source_dir: '/go/pkg/mod/cache' }] },
    { type: 'save_cache', key: 'cache-date-{{ year }}-{{ month }}-{{ day }}', contents: [{ source_dir: '/go/pkg/mod/cache' }] },
    { type: 'save_to_workspace', contents: [{ source_dir: './bin', dest_dir: '/bin/', paths: ['*'] }] },
  ],
};

local task_integration_tests(store, pgversion, arch) = {
  name: 'integration tests store: ' + store + ', postgres: ' + pgversion + ', arch: ' + arch,
  runtime: ci_runtime(pgversion, 'amd64'),
  environment: {
    STOLON_TEST_STORE_BACKEND: store,
    POSTGRES_PATH: '/usr/lib/postgresql/' + pgversion,
  },
  steps: [
    { type: 'restore_workspace', dest_dir: '.' },
    {
      type: 'run',
      name: 'test',
      command: |||
        export TMPDIR=/stolontemp
        export PATH=$POSTGRES_PATH:$PATH
        export BINDIR=${PWD}/bin
        export STKEEPER_BIN=${BINDIR}/stolon-keeper
        export STSENTINEL_BIN=${BINDIR}/stolon-sentinel
        export STPROXY_BIN=${BINDIR}/stolon-proxy
        export STCTL_BIN=${BINDIR}/stolonctl
        export ETCD_BIN="${HOME}/etcd/etcd"
        export CONSUL_BIN="${HOME}/consul"
        INTEGRATION=1 ./bin/integration-tests -test.parallel 2 -test.v
      |||,
    },
  ],
  depends: [
    'build go 1.14 ' + arch,
  ],
};

local task_build_push_images(name, pgversions, istag, push) =
  local imagebase = if istag then 'sorintlab/stolon:${AGOLA_GIT_TAG:-test}' else 'sorintlab/stolon:${AGOLA_GIT_BRANCH:-test}';
  {
    name: name,
    runtime: dind_runtime('amd64'),
    environment: {
      DOCKERAUTH: { from_variable: 'dockerauth' },
    },
    working_dir: '/stolon',
    steps: [
      { type: 'restore_workspace', dest_dir: '/stolon' },
      { type: 'run', command: 'apk add make' },
    ] + std.prune([
      if push then {
        type: 'run',
        name: 'generate docker auth',
        command: |||
          mkdir ~/.docker
          cat << EOF > ~/.docker/config.json
          {
            "auths": {
              "https://index.docker.io/v1/": { "auth" : "$DOCKERAUTH" }
            }
          }
          EOF
        |||,
      },
    ]) + [
      { type: 'run', command: 'for PGVERSION in %s; do make PGVERSION=${PGVERSION} TAG=%s-pg${PGVERSION} docker; done' % [pgversions, imagebase] },
    ] + std.prune([
      if push then { type: 'run', command: 'for PGVERSION in %s; do docker push %s-pg${PGVERSION}; done' % [pgversions, imagebase] },
    ]),
    depends: ['checkout code and save to workspace', 'integration tests store: etcdv3, postgres: 11, arch: amd64'],
  };

{
  runs: [
    {
      name: 'stolon build/test',
      tasks: [
        {
          name: 'checkout code and save to workspace',
          runtime: {
            arch: 'amd64',
            containers: [
              {
                image: 'alpine/git',
              },
            ],
          },
          steps: [
            { type: 'clone' },
            { type: 'save_to_workspace', contents: [{ source_dir: '.', dest_dir: '.', paths: ['**'] }] },
          ],
          depends: [],
        },
        {
          name: 'k8s test',
          runtime: {
            arch: 'amd64',
            containers: [
              {
                image: 'bsycorp/kind:v1.19.4',
                privileged: true,
                entrypoint: '/usr/bin/supervisord --nodaemon -c /etc/supervisord.conf',
              },
            ],
          },
          working_dir: '/stolon',
          steps: [
            { type: 'restore_workspace', dest_dir: '/stolon' },
            { type: 'run', command: 'apk add bash' },
            { type: 'run', command: './scripts/agola-k8s.sh' },
          ],
          depends: ['checkout code and save to workspace'],
        },
      ] + std.flattenArrays([
        [
          task_build_go(version, arch),
        ]
        for version in ['1.13', '1.14']
        for arch in ['amd64' /*, 'arm64' */]
      ]) + std.flattenArrays([
        [
          task_integration_tests(store, pgversion, 'amd64'),
        ]
        for store in ['etcdv2', 'consul']
        for pgversion in ['12']
      ]) + std.flattenArrays([
        [
          task_integration_tests(store, pgversion, 'amd64'),
        ]
        for store in ['etcdv3']
        for pgversion in ['9.5', '9.6', '10', '11', '12']
      ]) + [
        task_build_push_images('test build docker "stolon" images', '9.4 9.5 9.6 10 11 12', false, false)
        + {
          when: {
            branch: {
              include: ['#.*#'],
              exclude: ['master'],
            },
            ref: '#refs/pull/\\d+/head#',
          },
        },
        task_build_push_images('build and push docker "stolon" master branch images', '9.4 9.5 9.6 10 11 12', false, true)
        + {
          when: {
            branch: 'master',
          },
        },
        task_build_push_images('build and push docker "stolon" tag images', '9.4 9.5 9.6 10 11 12', true, true)
        + {
          when: {
            tag: '#v.*#',
          },
        },
      ],
    },
  ],
}
