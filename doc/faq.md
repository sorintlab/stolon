# FAQ

## Why clients should use the stolon proxy?

Since stolon by default leverages consistency over availability, there's the need for the clients to be connected to the current cluster elected master and be disconnected to unelected ones. For example, if you are connected to the current elected master and subsequently the cluster (for any valid reason, like network partitioning) elects a new master, to achieve consistency, the client needs to be disconnected from the old master (or it'll write data to it that will be lost when it resyncs). This is the purpose of the stolon proxy.

## Why didn't you use an already existing proxy like haproxy?

For our need to forcibly close connections to unelected masters and handle keepers/sentinel that can come and go and change their addresses we implemented a dedicated proxy that's directly reading it's state from the store. Thanks to go goroutines it's very fast.

We are open to alternative solutions (PRs are welcome) like using haproxy if they can met the above requirements. For example, an hypothetical haproxy based proxy needs a way to work with changing ip addresses, get the current cluster information and being able to forcibly close a connection when an haproxy backend is marked as failed (as a note, to achieve the latter, a possible solution that needs testing will be to use the [on-marked-down shutdown-sessions](https://cbonte.github.io/haproxy-dconv/configuration-1.6.html#5.2-on-marked-down) haproxy server option).

## Does the stolon proxy sends read-only requests to standbys?

Currently the proxy redirects all requests to the master. There is a [feature request](https://github.com/sorintlab/stolon/issues/132) for using the proxy also for standbys but it's low in the priority list.

If your application want to query the hot standbys, currently you can read the standby dbs and their status form the cluster data directly from the store (but be warned that this isn't meant to be stable).

## Why is shared storage and fencing not necessary with stolon?

stolon eliminates the requirement of a shared storage since it uses postgres streaming replication and can avoid the need of fencing (killing the node, removing access to the shared storage etc...) due to its architecture:
* It uses etcd/consul as the first step to determine which components are healthy.
* The stolon-proxy is a sort of fencer since it'll close connections to old masters and direct new connections to the current master.

## How does stolon differ from pure kubernetes high availability?

A pure kubernetes approach to achieve postgres high availability is using persistent volumes and statefulsets (petsets). By using persistent volumes means you won't lose any transaction. k8s currently requires fencing to avoid data corruption when using statefulsets with persistent volumes (see https://github.com/kubernetes/kubernetes/pull/34160).

stolon instead uses postgres streaming replication to achieve high availability. To avoid losing any transaction you can enable synchronous replication.

With stolon you also have other pros:

* Currently k8s failed node detection/pod eviction takes minutes while stolon will by default detects failures in seconds.
* On cloud providers like aws, an ebs volume is tied to an availability zone so you cannot failover to another availability zone. With stolon you can.
* You can tie instance data to a specific node if you don't want to use persistent volumes like aws ebs.
* You can easily deploy and manage new clusters
 * update the cluster spec
  * update postgres parameters
* Do point in time recovery
* Create master/standby stolon clusters (future improvement).
* Easily scale stolon components.

## How does stolon differ from pacemaker?

With pacemaker, using the provided postgresql resource agent, you can achieve instance high availability and can choose between using shared storage or streaming replication.

The stolon pros are the same of the above question and it'll let you avoid the need of node fencing and provides an easier cluster management and scaling.

## What happens if etcd/consul is partitioned?

See [Stolon Architecture and Requirements](doc/architecture.md)

## How are backups handled with stolon?

stolon let you easily integrate with any backup/restore solution. See the [point in time recovery](pitr.md) and the [wal-e example](pitr_wal-e.md).

## How stolon decide which standby should be promoted to master?

When using async replication the leader sentinel tries to find the best standby using a valid standby with the (last reported) nearest xlog location to the master latest knows xlog location. If a master is down there's no way to know its latest xlog position (stolon get and save it at some intervals) so there's no way to guarantee that the standby is not behind but just that the best standby of the ones available will be choosen.

When using synchronous replication only synchronous standbys will be choosen so standbys behind the master won't be choosen (be aware of postgresql synchronous replication limits explaned in the [postgresql documentation](https://www.postgresql.org/docs/9.6/static/warm-standby.html#SYNCHRONOUS-REPLICATION), for example, when a master restarts while no synchronous standbys are available, the transactions waiting for acknowledgement on the master will be marked as fully committed. We are thinking of a way to avoid this using stolon).

## Does stolon use Consul as a DNS server as well?

Consul (or etcd) is used only as a key-value storage.

## Lets say I have multiple stolon clusters. Do I need a separate stores (etcd or consul) for each stolon cluster?

stolon will create its keys using the cluster name as part of the key hierarchy. So multiple stolon clusters can share the same store.

The suggestion is to use a store located in the same *region*/*datacenter* (the concepts are related on your architecture/cloud provider) with the stolon cluster.
