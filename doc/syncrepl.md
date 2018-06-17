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
