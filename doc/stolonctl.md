## Stolon Client (stolonctl)

`stolonctl` is the stolon client which controls the stolon cluster(s)

It needs to communicate with the etcd cluster (`--etcd-endpoints`) on which the requested cluster name (`--cluster-name`) is running and for certain commands with an active leader sentinel.

To avoid repeating for every command (or inside scripts) all the options you can export them as environment variables. Their name will be the same as the option name converted in uppercase, with `_` replacing `-` and prefixed with `STOLONCTL_`.

Ex.
```
STOLONCTL_ETCD_ENDPOINTS
STOLONCTL_CLUSTER_NAME
```


### status ###

Retrieve the current cluster status

```
stolonctl --cluster-name mycluster status
=== Active sentinels ===

ID              LISTENADDRESS   LEADER
2051827f        localhost:6431  true

=== Active proxies ===

ID              LISTENADDRESS   CV VERSION
fc6b8f04        127.0.0.1:25432 43

=== Keepers ===

ID              LISTENADDRESS   PG LISTENADDRESS        CV VERSION      HEALTHY
postgres0       localhost:5431  localhost:5432          43              true
postgres1       localhost:5433  localhost:5435          41              false

=== Required Cluster View ===

Version: 43
Master: postgres0

===== Keepers tree =====

postgres0 (master)
└─postgres1

```

### list-clusters ###

List all the cluster available under the default etcd base path

### config ###

See [cluster config](cluster_config.md)
### config get ###

Get the current cluster config

### config replace ###

Replace the current cluster config

### config patch ###

Patch the current cluster config
