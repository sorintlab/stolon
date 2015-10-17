# Synchronous replication

**Note:** this is temporary. In future you can update the cluster config using a stolon client (`stolonctl`) instead of manually writing the full cluster config inside etcd since this is quite error prone.

You can enable/disable synchronous replication at any time and the keepers will reconfigure themselves.

To do this, you should write the cluster config to the etcd path: `/stolon/cluster/$CLUSTERNAME/config` using for example `curl` or `etcdctl`.

## Enable synchronous replication.

Assuming that you cluster name is `mycluster` and etcd is listening on localhost:2379:
```
curl http://127.0.0.1:2379/v2/keys/stolon/cluster/mycluster/config -XPUT -d value='{ "synchronousreplication" : true }'
```

or with etcdctl
```
etcdctl set /stolon/cluster/stolon-cluster/config '{ "synchronousreplication" : true }'
```
