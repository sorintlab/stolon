## Stolon Client (stolonctl)

`stolonctl` is the stolon client which controls the stolon cluster(s)

It needs to communicate with the cluster store (providing `--store-backend` and `--store-endpoints` options) on which the requested cluster name (`--cluster-name`) is running and for certain commands with an active leader sentinel.

To avoid repeating for every command (or inside scripts) all the options you can export them as environment variables. Their name will be the same as the option name converted in uppercase, with `_` replacing `-` and prefixed with `STOLONCTL_`.

Ex.
```
STOLONCTL_STORE_BACKEND
STOLONCTL_STORE_ENDPOINTS
STOLONCTL_CLUSTER_NAME
```


### status ###

Retrieve the current cluster status

```
stolonctl --cluster-name mycluster status
=== Active sentinels ===

ID              LEADER
2051827f        true

=== Active proxies ===

ID
fc6b8f04

=== Keepers ===

UID             LISTENADDRESS   PG LISTENADDRESS        HEALTHY PGWANTEDGENERATION      PGCURRENTGENERATION
postgres0       localhost:5431  localhost:5432          true    3                       3
postgres1       localhost:5433  localhost:5435          true    4                       4

=== Required Cluster ===

Master: postgres0

===== Keepers tree =====

postgres0 (master)
└─postgres1

```

### init ###

Initialize a new cluster
See [initialization](initialization.md)

### update ###

Update a cluster specification
See [cluster spec](cluster_spec.md)

### spec  ###

Get the current cluster specification

