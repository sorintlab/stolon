# stolon - PostgreSQL cloud native HA replication manager

[![Build Status](https://semaphoreci.com/api/v1/projects/fb01aecd-c3d5-407b-a157-7d5365e9e4b6/565617/badge.svg)](https://semaphoreci.com/sorintlab/stolon)
[![Join the chat at https://gitter.im/sorintlab/stolon](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/sorintlab/stolon?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

stolon is a cloud native PostgreSQL manager for PostgreSQL high availability. It's cloud native because it'll let you keep an high available PostgreSQL inside your containers (kubernetes integration) but also on every other kind of infrastructure (cloud IaaS, old style infrastructures etc...)

## Features

* Leverages PostgreSQL streaming replication.
* [kubernetes integration](examples/kubernetes/README.md) letting you achieve postgreSQL high availability.
* Uses [etcd](https://github.com/coreos/etcd) as an high available data store and for leader election
* Asynchronous (default) and [synchronous](doc/syncrepl.md) replication.
* Full cluster setup in minutes.
* Easy [cluster admininistration](doc/stolonctl.md)

## Architecture

Stolon is composed of 3 main components

* keeper: it manages a PostgreSQL instance converging to the clusterview provided by the sentinel(s).
* sentinel: it discovers and monitors keepers and calculates the optimal clusterview.
* proxy: the client's access point. It enforce connections to the right PostgreSQL master and forcibly closes connections to unelected masters.

![Stolon architecture](doc/architecture_small.png)

## Project Status

Stolon is under active development and used in different environments. But its on disk format (etcd hierarchy and key contents) is not stable and will change for introducing new features. We hope that at the end of the year (2015) the most breaking changes will be merged and we can commit to a stable on disk format.

## Requirements

* PostgreSQL >= 9.4
* etcd >= 2.0


## build

```
./build
```

## Quick start and examples

* [simple cluster example](doc/simplecluster.md)
* [kubernetes example](examples/kubernetes/README.md)

## Documentation

* [stolon client (stolonctl)](doc/stolonctl.md)
* [cluster configuration](doc/cluster_config.md)

## High availability

Stolon tries to be resilent to any partitioning problem. The cluster view is computed by the leader sentinel and is useful to avoid data loss (one example over all avoid that old dead masters coming back are elected as the new master).

There can be tons of different partitioning cases. The primary ones are covered (and in future more will be added) by various [integration tests](tests/integration)

## Contributing to stolon

stolon is an open source project under the Apache 2.0 license, and contributions are gladly welcomed!
