## Setting PostgreSQL server parameters

The unique central point for managing the postgresql parameters is the [cluster_specification](cluster_spec.md) `pgParameters` map. This makes easy to centrally manage and keep in sync the postgresql paramaters of all the db instances in the cluster. The keepers will generate a `postgresql.conf` that contains the parameters defined in the [cluster_specification](cluster_spec.md) `pgParameters` map.

The user can change the parameters at every time and the keepers will update the `postgresql.conf` and reload the instance.

If some parameters needs an instance restart to be applied this should be manually done by the user restarting the keepers.

### Ignored parameters

These parameters, if defined in the cluster specification, will be ignored since they are managed by stolon and cannot be defined by the user:

```
listen_addresses
port
unix_socket_directories
wal_keep_segments
wal_log_hints
hot_standby
max_replication_slots
max_wal_senders
synchronous_standby_names
```

## Special cases

### wal_level

since stolon requires a `wal_level` value of at least `replica` (or `hot_standby` for pg < 9.6) if you leave it unspecificed in the `pgParameters` or if you specify a wrong `wal_level` or a value lesser than `replica` or `hot_standby` (like `minimal`) it'll be overridden by the minimal working value (`replica` or `hot_standby`).

i.e. if you want to also save logical replication information in the wal files you can specify a `wal_level` set to `logical`.

## Parameters validity checks

Actually stolon doesn't do any check on the provided configurations, so, if the provided parameters are wrong this won't create problems at instance reload (just some warning in the postgresql logs) but at the next instance restart, it'll probably fail making the instance not available (thus triggering failover if it's the master or other changes in the clusterview).

## Initialization parameters

When [initializing the cluster](initialization.md), by default, stolon will merge in the cluster spec the parameters that the instance had at the end of the initialization, practically:

* When initMode is new, it'll merge the initdb generated parameters.
* When initMode is existing it'll merge the parameters of the existing instance.

To disable this behavior just set `mergePgParameters` to false in the cluster spec and manually set all the pgParameters in the cluster specification.

## postgresql.auto.conf and ALTER SYSTEM commands

Since postgresql.auto.conf overrides postgresql.conf parameters, changing some of them with ALTER SYSTEM could break the cluster (parameters managed by stolon could be overridden) and make pg parameters different between the instances.

To avoid this stolon disables the execution of ALTER SYSTEM commands making postgresql.auto.conf a symlink to /dev/null. When an ALTER SYSTEM command is executed it'll return an error.

## Restart postgres on changing some pg parameters

There are some pg parameters which requires postgres restart to take effect after changing. For example, changing `max_connections` will not take effect till the underlying postgres is restarted. This is disabled by default and can be enabled using the clusterSpecification `automaticPgRestart`. This is achieved using `pending_restart` in [pg_settings](https://www.postgresql.org/docs/9.5/static/view-pg-settings.html) for postgres 9.5 and above and the `context` column of `pg_settings` for lower versions (<= 9.4).