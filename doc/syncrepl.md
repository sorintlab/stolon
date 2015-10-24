# Synchronous replication

You can enable/disable synchronous replication at any time and the keepers will reconfigure themselves.

You can do this with `stolonctl`.

## Enable synchronous replication.

Assuming that you cluster name is `mycluster` and etcd is listening on localhost:2379:
```
stolonctl --cluster-name=mycluster config patch '{ "synchronousreplication" : true }'
```

## Disable synchronous replication.

```
stolonctl --cluster-name=mycluster config patch '{ "synchronousreplication" : false }'
```
