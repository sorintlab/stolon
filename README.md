# stolon - PostgreSQL cloud native HA replication manager

[![Build Status](https://semaphoreci.com/api/v1/projects/fb01aecd-c3d5-407b-a157-7d5365e9e4b6/565617/badge.svg)](https://semaphoreci.com/sorintlab/stolon)
[![Join the chat at https://gitter.im/sorintlab/stolon](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/sorintlab/stolon?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

stolon is a cloud native PostgreSQL manager for PostgreSQL high availability. It's cloud native because it'll let you keep an high available PostgreSQL inside your containers (kubernetes integration) but also on every other kind of infrastructure (cloud IaaS, old style infrastructures etc...)

For an introduction to stolon you can also take a look at [this post](https://sgotti.me/post/stolon-introduction/)

## Features

* Leverages PostgreSQL streaming replication.
* Resilient to any kind of partitioning. While trying to keep the maximum availability, it prefers consistency over availability.
* [kubernetes integration](examples/kubernetes/README.md) letting you achieve postgreSQL high availability.
* Uses a cluster store like [etcd](https://github.com/coreos/etcd) or [consul](https://www.consul.io) as an high available data store and for leader election
* Asynchronous (default) and [synchronous](doc/syncrepl.md) replication.
* Full cluster setup in minutes.
* Easy [cluster admininistration](doc/stolonctl.md)
* Automatic service discovery and dynamic reconfiguration (handles postgres and stolon processes changing their addresses).
* Can use [pg_rewind](doc/pg_rewind.md) for fast instance resyncronization with current master.

## Architecture

Stolon is composed of 3 main components

* keeper: it manages a PostgreSQL instance converging to the clusterview provided by the sentinel(s).
* sentinel: it discovers and monitors keepers and calculates the optimal clusterview.
* proxy: the client's access point. It enforce connections to the right PostgreSQL master and forcibly closes connections to unelected masters.

![Stolon architecture](doc/architecture_small.png)

## Project Status

Stolon is under active development and used in different environments. Probably its on disk format (store hierarchy and key contents) will change in future to support new features. If a breaking change is needed it'll be documented in the release notes and an upgrade path will be provided.

Anyway it's quite easy to reset a cluster from scratch keeping the current master instance working and without losing any data.

## Requirements

* PostgreSQL >= 9.4
* etcd >= 2.0 or consul >=0.6


## build

```
./build
```

## test

Basic go tests can be launched like:
```
./test
```

Also you might want to launch [integration tests](tests/integration) locally.
These tests use some real backends, in this examle I'll use etcd and postgresql 9.4.
If you are capable to run stolon locally, for example using [simple cluster example](doc/simplecluster.md), you will launch tests like:
```sh
$ PATH=/usr/lib/postgresql/9.4/bin/:$PATH INTEGRATION=1 STOLON_TEST_STORE_BACKEND=etcd ETCD_BIN=$GOPATH/src/github.com/coreos/etcd/bin/etcd ./test
```

Tests can leave demonized postgresql processes, to be on the safe side check it with:
```sh
$ pidof postgres
# Kill if needed
$ sudo kill -TERM $(pidof postgres)

Also your postgres service should be stopped before this.
Check it with:
```sh
$ sudo systemctl status postgresql
# Stop if needed
$ sudo systemctl stop postgresql
```

## Quick start and examples

* [simple cluster example](doc/simplecluster.md)
* [kubernetes example](examples/kubernetes/README.md)

## Documentation

* [stolon client (stolonctl)](doc/stolonctl.md)
* [cluster configuration](doc/cluster_config.md)

## High availability

Stolon tries to be resilient to any partitioning problem. The cluster view is computed by the leader sentinel and is useful to avoid data loss (one example over all avoid that old dead masters coming back are elected as the new master).

There can be tons of different partitioning cases. The primary ones are covered (and in future more will be added) by various [integration tests](tests/integration)

## FAQ

### Why clients should use the stolon proxy?

Since stolon by default leverages consistency over availability, there's the need for the clients to be connected to the current cluster elected master and be disconnected to unelected ones. For example, if you are connected to the current elected master and subsequently the cluster (for any valid reason, like network partitioning) elects a new master, to achieve consistency, the client needs to be disconnected from the old master (or it'll write data to it that will be lost when it resyncs). This is the purpose of the stolon proxy.

### Why didn't you use an already existing proxy like haproxy?

For our need to forcibly close connections to unelected masters and handle keepers/sentinel that can come and go and change their addresses we implemented a dedicated proxy that's directly reading it's state from the store. Thanks to go goroutines it's very fast.

We are open to alternative solutions (PRs are welcome) like using haproxy if they can met the above requirements. For example, an hypothetical haproxy based proxy needs a way to work with changing ip addresses, get the current cluster information and being able to forcibly close a connection when an haproxy backend is marked as failed (as a note, to achieve the latter, a possible solution that needs testing will be to use the [on-marked-down shutdown-sessions](https://cbonte.github.io/haproxy-dconv/configuration-1.6.html#5.2-on-marked-down) haproxy server option).

## Contributing to stolon

stolon is an open source project under the Apache 2.0 license, and contributions are gladly welcomed!
