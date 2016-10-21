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

### Does proxy send read-only requests to standbys?

Currently the proxy redirects all requests to the master. There is a [feature request](https://github.com/sorintlab/stolon/issues/132) for using the proxy also for standbys but it's low in the priority list. There is a workaround though.

Application can learn cluster configuration from `stolon/cluster/mycluster/clusterdata` key. Consul allows to subscribe to updates of this key like this: 

```
http://localhost:8500/v1/kv/stolon/cluster/mycluster/clusterdata?wait=0s&index=14379
```

... where 14379 is ModifyIndex of a key reported by Consul.

### How stolon decide which standby should be promoted to master?

Currently it tries to find the best standby, the one with the xlog location nearest to the master latest knows xlog location. If a master is down there's no way to know its latest xlog position (stolon get and save it at some intervals) so there's no way to guarantee that the standby is not behind but just that the best standby of the ones available will be choosen. 

### Does synchronous replication mean that I can't loose any data?

Since version 9.6 PostgreSQL supports [synchronous replication to the quorum](https://www.postgresql.org/docs/9.6/static/runtime-config-replication.html#GUC-SYNCHRONOUS-STANDBY-NAMES). Unfortunately stolon doesn't support this feature yet and configures replication like this:

```
# on postgres1 server
synchronous_standby_names = 'postgres2,postgres3'
```

According to PostgreSQL documentation:

> Specifies a comma-separated list of standby names that can support synchronous replication, as described in Section 25.2.8. At any one time there will be at most one active synchronous standby; transactions waiting for commit will be allowed to proceed after this standby server confirms receipt of their data. The synchronous standby will be the first standby named in this list that is both currently connected and streaming data in real-time (as shown by a state of streaming in the pg\_stat\_replication view). Other standby servers appearing later in this list represent potential synchronous standbys.

It means that in case of netsplit synchronous standby can be not among majority nodes. In this case some recent changes will be lost. Although it's not a major problem for most web projects, currently you shouldn't use stolon for storing data that under no circumstances can't be lost.

### Does stolon use Consul as a DNS server as well?

Consul (or etcd) is used only as a key-value storage.

### Lets say I have multiple stolon clusters. Do I need a separate Consul / etcd cluster for each stolon cluster?

It depends on your architecture and where the different stolon clusters are located. In general, if two clusters live on complitely different hardware, to to handle all possible courner cases (like netslits) you need a separate Consul / etcd cluster for each stolon cluster.


## Contributing to stolon

stolon is an open source project under the Apache 2.0 license, and contributions are gladly welcomed!
