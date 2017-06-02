# stolon - PostgreSQL cloud native High Availability

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
* Can do point in time recovery integrating with your preferred backup/restore tool.
* [Standby cluster](doc/standbycluster.md) (for multi site replication and near zero downtime migration).
* Automatic service discovery and dynamic reconfiguration (handles postgres and stolon processes changing their addresses).
* Can use [pg_rewind](doc/pg_rewind.md) for fast instance resyncronization with current master.

## Architecture

Stolon is composed of 3 main components

* keeper: it manages a PostgreSQL instance converging to the clusterview provided by the sentinel(s).
* sentinel: it discovers and monitors keepers and calculates the optimal clusterview.
* proxy: the client's access point. It enforce connections to the right PostgreSQL master and forcibly closes connections to unelected masters.

![Stolon architecture](doc/architecture_small.png)

## Documentation

[Documentation Index](doc/README.md)

## Quick start and examples

* [simple cluster example](doc/simplecluster.md)
* [kubernetes example](examples/kubernetes/README.md)

## Project Status

Stolon is under active development and used in different environments. Probably its on disk format (store hierarchy and key contents) will change in future to support new features. If a breaking change is needed it'll be documented in the release notes and an upgrade path will be provided.

Anyway it's quite easy to reset a cluster from scratch keeping the current master instance working and without losing any data.

## Requirements

* PostgreSQL 10 or 9 (9.4, 9.5, 9.6)
* etcd >= 2.0 or consul >=0.6


## build

```
./build
```

## High availability

Stolon tries to be resilient to any partitioning problem. The cluster view is computed by the leader sentinel and is useful to avoid data loss (one example over all avoid that old dead masters coming back are elected as the new master).

There can be tons of different partitioning cases. The primary ones are covered (and in future more will be added) by various [integration tests](tests/integration)

## FAQ

See [here](doc/faq.md) for a list of faq. If you have additional questions please ask.

### How does Stolon differ from ____?

#### Pure Kubernetes

Stolon does not require a shared volume be attached to whatever node the Postgres container is running on. **Is Stolon faster than K8's pod process?? Is that another reason? I can't imagine it would be that different**. With a pure Kubernetes approach and something like an EBS volume, you can have a single Postgres master and even slave nodes with replication that could serve as read only servers. In the event of a master failure, K8S can move the DB container around and repoint the service as necessary.

#### Pacemaker

Stolon eliminates the fencing/shared storage requirement because:
1. It can rely on etcd/consul clustering in the event of partitions
2. It makes more assumptions (consistency) about what to do in these events (see the `testTimelineFork` integration test for some examples.)

### Why is fencing not necessary with Stolon?

First, no shared storage. Then we can have multiple cases of partitioning. So we try to solve this primarily in two ways: 1) The sentinel computes a "wanted" component states (now called cluster view). The keepers and the proxies try to converge to this states. 2) The stolon-proxy is the one that forces clients to the right master and forcibly closes connection to unelected ones. So you have to connect to the master only via the proxy. So, if all the stolon components behave correctly we can avoid fencing.

### What happens if etcd is partitioned?

Etcd isn't a problem. Due to Raft it'll only work with a quorum so there's no way to write to partition without quorum. In this case all the stolon will get an error and then retry in the next interval. Additionally the stolon-proxy, if it cannot talk with etcd, by default will drop all connection to the master since it cannot know if the cluster view has changed (for example if the proxy has problems talking with etcd but the sentinel can talk).

### How are backups handled with Stolon?

**TBD. Explain a little about WAL-E, how to configure Stolon to work with it and what happens when partitions, etc. occur.**

## Contributing to stolon

stolon is an open source project under the Apache 2.0 license, and contributions are gladly welcomed!
To submit your changes please open a pull request.

## Contacts

* For bugs and feature requests file an [issue](https://github.com/sorintlab/stolon/issues/new)
* For general discussion about using and developing stolon, join the [stolon](https://groups.google.com/forum/#!forum/stolon) mailing list
* For real-time discussion, join us on [Gitter](https://gitter.im/sorintlab/stolon)
