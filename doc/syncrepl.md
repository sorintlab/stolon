# Synchronous replication

You can enable/disable synchronous replication at any time and the keepers will reconfigure themselves using `stolonctl update` to update the [cluster specification](cluster_spec.md).

## Enable synchronous replication.

Assuming that your cluster name is `mycluster` and using etcd listening on localhost:2379:
```
stolonctl --cluster-name=mycluster --store-backend=etcd update --patch '{ "synchronousReplication" : true }'
```

## Disable synchronous replication.

```
stolonctl --cluster-name=mycluster --store-backend=etcd update --patch '{ "synchronousReplication" : false }'
```
