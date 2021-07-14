module github.com/sorintlab/stolon

require (
	github.com/cockroachdb/datadriven v0.0.0-20190809214429-80d97fb3cbaa // indirect
	github.com/coreos/go-systemd v0.0.0-20180511133405-39ca1b05acc7 // indirect
	github.com/coreos/pkg v0.0.0-20160727233714-3ac0863d7acf // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/leadership v0.1.0
	github.com/docker/libkv v0.2.1
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/golang/mock v1.4.0
	github.com/google/go-cmp v0.5.5
	github.com/gorilla/websocket v0.0.0-20170926233335-4201258b820c // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.1-0.20190118093823-f849b5445de4 // indirect
	github.com/hashicorp/consul/api v1.4.0
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/lib/pq v1.3.0
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-runewidth v0.0.2 // indirect
	github.com/mitchellh/copystructure v1.0.0
	github.com/olekukonko/tablewriter v0.0.0-20170122224234-a0225b3f23b5 // indirect
	github.com/prometheus/client_golang v1.11.0
	github.com/satori/go.uuid v1.2.0
	github.com/sgotti/gexpect v0.0.0-20210315095146-1ec64e69809b
	github.com/soheilhy/cmux v0.1.4 // indirect
	github.com/sorintlab/pollon v0.0.0-20181009091703-248c68238c16
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/tmc/grpc-websocket-proxy v0.0.0-20170815181823-89b8d40f7ca8 // indirect
	github.com/urfave/cli v1.20.0 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.etcd.io/etcd v3.3.25+incompatible
	go.etcd.io/etcd/api/v3 v3.5.0 // indirect
	go.etcd.io/etcd/client/v3 v3.5.0
	go.uber.org/tools v0.0.0-20190618225709-2cfd321de3ee // indirect
	go.uber.org/zap v1.17.0
	golang.org/x/tools v0.1.2 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.25 // indirect
	gopkg.in/resty.v1 v1.12.0 // indirect
	honnef.co/go/tools v0.0.1-2019.2.3 // indirect
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
)

go 1.12

replace github.com/coreos/bbolt v1.3.3 => github.com/etcd-io/bbolt v1.3.3
