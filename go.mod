module github.com/sorintlab/stolon

require (
	// force github.com/coreos/etcd to v3.3.18 that doesn't use github.com/ugorji/go
	github.com/coreos/etcd v3.3.18+incompatible // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/leadership v0.1.0
	github.com/docker/libkv v0.2.1
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/golang/mock v1.4.0
	github.com/google/go-cmp v0.4.0
	github.com/hashicorp/consul/api v1.4.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/lib/pq v1.3.0
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/copystructure v1.0.0
	github.com/prometheus/client_golang v1.4.1
	github.com/satori/go.uuid v1.2.0
	github.com/sgotti/gexpect v0.0.0-20161123102107-0afc6c19f50a
	github.com/sorintlab/pollon v0.0.0-20181009091703-248c68238c16
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	go.uber.org/zap v1.13.0
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
)

go 1.12
