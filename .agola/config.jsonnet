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
      image: 'sorintlab/stolon-ci-image:v0.1.0-pg' + pgversion,
      volumes: [
        {
          path: '/stolontemp',
          tmpfs: {},
        },
      ],
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
    'build go 1.13 ' + arch,
  ],
};

{
  runs: [
    {
      name: 'stolon build/test',
      tasks: std.flattenArrays([
        [
          task_build_go(version, arch),
        ]
        for version in ['1.12', '1.13']
        for arch in ['amd64' /*, 'arm64' */]
      ]) + std.flattenArrays([
        [
          task_integration_tests(store, pgversion, 'amd64'),
        ]
        for store in ['etcdv2', 'consul']
        for pgversion in ['11' /*, '12' */]
      ]) + std.flattenArrays([
        [
          task_integration_tests(store, pgversion, 'amd64'),
        ]
        for store in ['etcdv3']
        for pgversion in ['9.5', '9.6', '10', '11' /*, '12' */]
      ]),
    },
  ],
}
