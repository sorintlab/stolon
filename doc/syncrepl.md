# Synchronous replication

Since synchronous replication is usually needed to avoid losing some transactions, stolon implements it in a way to avoid any possibility of electing non sync standbys as new masters.
When synchronous replication is enabled stolon will always ensure that a master has N synchronous standbys (where N will be between MinSynchronousStandbys and MaxSynchronousStandbys values defined in the [cluster specification](cluster_spec.md)). If there're not enough available standbys then it will also add a fake standby server in the `synchronous_standby_names`. Adding a non existing standby server will ensure the master will always block waiting for remote commits.

You can enable/disable synchronous replication at any time and the keepers will reconfigure themselves using `stolonctl update` to update the [cluster specification](cluster_spec.md).

### Min and Max number of synchronous replication standbys

In the cluster spec you can set the `MinSynchronousStandbys` and `MaxSynchronousStandbys` values (they both defaults to 1). Having multiple synchronous standbys is a feature provided starting from [PostgreSQL 9.6](https://www.postgresql.org/docs/9.6/static/warm-standby.html#SYNCHRONOUS-REPLICATION). Values different than 1 for postgres versions below 9.6 will be ignored.

## Enable synchronous replication.

Assuming that your cluster name is `mycluster` and using etcd listening on localhost:2379:
```
stolonctl --cluster-name=mycluster --store-backend=etcd update --patch '{ "synchronousReplication" : true }'
```

## Disable synchronous replication.

```
stolonctl --cluster-name=mycluster --store-backend=etcd update --patch '{ "synchronousReplication" : false }'
```

## Set min and max number of synchronous replication standbys

Set MinSynchronousStandbys/MaxSynchronousStandbys to a value greater than 1 (only when using PostgreSQL >= 9.6)

```
stolonctl --cluster-name=mycluster --store-backend=etcd update --patch '{ "synchronousReplication" : true, "minSynchronousStandbys": 2, "maxSynchronousStandbys": 3 }'
```

## Handling postgresql sync repl limits under such circumstances

Postgres synchronous replication has a downside explained in the [docs](https://www.postgresql.org/docs/current/static/warm-standby.html)

`If primary restarts while commits are waiting for acknowledgement, those waiting transactions will be marked fully committed once the primary database recovers. There is no way to be certain that all standbys have received all outstanding WAL data at time of the crash of the primary. Some transactions may not show as committed on the standby, even though they show as committed on the primary. The guarantee we offer is that the application will not receive explicit acknowledgement of the successful commit of a transaction until the WAL data is known to be safely received by all the synchronous standbys.`

Under some events this will cause lost transactions. For example:

* Sync standby goes down.
* A client commits a transaction, it blocks waiting for acknowledgement.
* Primary restart, it'll mark the above transaction as fully committed. All the
clients will now see that transaction.
* Primary dies
* Standby comes back.
* The sentinel will elect the standby as the new master since it's in the
synchronous_standby_names list.
* The above transaction will be lost despite synchronous replication being
enabled.

So there can be some conditions where a syncstandby could be elected also if it's missing the last transactions if it was down at the commit time.

It's not easy to fix this issue since these events cannot be resolved by the sentinel because it's not possible to know if a sync standby is really in sync when the master is down (since we cannot query its last wal position and the reporting from the keeper is asynchronous).

But with stolon we have the power to overcome this issue by noticing when a primary restarts (since we control it), allow only "internal" connections until all the defined synchronous standbys are really in sync.

Allowing only "internal" connections means not adding the default rules or the user defined pgHBA rules but only the rules needed for replication (and local communication from the keeper).

Since "internal" rules accepts the defined superuser and replication users, client should not use these roles for normal operation or the above solution won't work (but they shouldn't do it anyway since this could cause exhaustion of reserved superuser connections needed by the keeper to check the instance).
