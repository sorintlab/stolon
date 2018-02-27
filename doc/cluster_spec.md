## Cluster Specification ##

Stolon has a declarative model where you specify the desired cluster state, this is called cluster specification and it's saved inside the cluster data in the store.

A cluster needs to be initialized providing a cluster specification.
This can be achieved using `stolonctl init`.

A cluster specification is updatable using `stolonctl update`.

Some options in a running cluster specification can be changed to update the desired state. Sometimes a cluster state can be updated only in some directions, this means that some options cannot be updated on a running cluster but will require a new cluster initialization.


### Cluster Specification Format.

| Name                      | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Required                  | Type              | Default                                                                                                                             |
|---------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------|-------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| sleepInterval             | interval to wait before next check (for every component: keeper, sentinel, proxy).                                                                                                                                                                                                                                                                                                                                                                                                | no                        | string (duration) | 5s                                                                                                                                  |
| requestTimeout            | time after which any request (keepers checks from sentinel etc...) will fail.                                                                                                                                                                                                                                                                                                                                                                                                     | no                        | string (duration) | 10s                                                                                                                                 |
| failInterval              | interval after the first fail to declare a keeper as not healthy.                                                                                                                                                                                                                                                                                                                                                                                                                 | no                        | string (duration) | 20s                                                                                                                                 |
| deadKeeperRemovalInterval | interval after which a dead keeper will be removed from the cluster data                                                                                                                                                                                                                                                                                                                                                                                                          | no                        | string (duration) | 48h                                                                                                                                 |
| maxStandbys               | max number of standbys. This needs to be greater enough to cover both standby managed by stolon and additional standbys configured by the user. Its value affect different postgres parameters like max_replication_slots and max_wal_senders. Setting this to a number lower than the sum of stolon managed standbys and user managed standbys will have unpredicatable effects due to problems creating replication slots or replication problems due to exhausted wal senders. | no                        | uint16            | 20                                                                                                                                  |
| maxStandbysPerSender      | max number of standbys for every sender. A sender can be a master or another standby (with cascading replication).                                                                                                                                                                                                                                                                                                                                                                | no                        | uint16            | 3                                                                                                                                   |
| maxStandbyLag             | maximum lag (from the last reported master state, in bytes) that an asynchronous standby can have to be elected in place of a failed master.                                                                                                                                                                                                                                                                                                                                      | no                        | uint32            | 1MiB                                                                                                                                |
| synchronousReplication    | use synchronous replication between the master and its standbys                                                                                                                                                                                                                                                                                                                                                                                                                   | no                        | bool              | false                                                                                                                               |
| minSynchronousStandbys    | minimum number of required synchronous standbys when synchronous replication is enabled (only set this to a value > 1 when using PostgreSQL >= 9.6)                                                                                                                                                                                                                                                                                                                               | no                        | uint16            | 1                                                                                                                                   |
| maxSynchronousStandbys    | maximum number of required synchronous standbys when synchronous replication is enabled (only set this to a value > 1 when using PostgreSQL >= 9.6)                                                                                                                                                                                                                                                                                                                               | no                        | uint16            | 1                                                                                                                                   |
| additionalWalSenders      | number of additional wal_senders in addition to the ones internally defined by stolon, useful to provide enough wal senders for external standbys (changing this value requires an instance restart)                                                                                                                                                                                                                                                                              | no                        | uint16            | 5                                                                                                                                   |
| additionalMasterReplicationSlots | a list of additional replication slots to be created on the master postgres instance. Replication slots not defined here will be dropped from the master instance (i.e. manually created replication slots will be removed).                                                                                                                                                                                                                                               | no                        | []string          | null                                                                                                                                |
| usePgrewind               | try to use pg_rewind for faster instance resyncronization.                                                                                                                                                                                                                                                                                                                                                                                                                        | no                        | bool              | false                                                                                                                               |
| initMode                  | The cluster initialization mode. Can be *new* or *existing*. *new* means that a new db cluster will be created on a random keeper and the other keepers will sync with it. *existing* means that a keeper (that needs to have an already created db cluster) will be choosed as the initial master and the other keepers will sync with it. In this case the `existingConfig` object needs to be populated.                                                                       | yes                       | string            |                                                                                                                                     |
| existingConfig            | configuration for initMode of type "existing"                                                                                                                                                                                                                                                                                                                                                                                                                                     | if initMode is "existing" | ExistingConfig    |                                                                                                                                     |
| mergePgParameters         | merge pgParameters of the initialized db cluster, useful the retain initdb generated parameters when InitMode is new, retain current parameters when initMode is existing or pitr.                                                                                                                                                                                                                                                                                                | no                        | bool              | true                                                                                                                                |
| role                      | cluster role (master or standby)                                                                                                                                                                                                                                                                                                                                                                                                                                                  | no                        | bool              | master                                                                                                                              |
| defaultSUReplAccessMode   | mode for the default hba rules used for replication by standby keepers (the su and repl auth methods will be the one provided in the keeper command line options). Values can be *all* or *strict*. *all* allow access from all ips, *strict* restrict master access to standby servers ips.                                                                                                                                                                                      | no                        | string            | all                                                                                                                                 |
| newConfig                 | configuration for initMode of type "new"                                                                                                                                                                                                                                                                                                                                                                                                                                          | if initMode is "new"      | NewConfig         |                                                                                                                                     |
| pitrConfig                | configuration for initMode of type "pitr"                                                                                                                                                                                                                                                                                                                                                                                                                                         | if initMode is "pitr"     | PITRConfig        |                                                                                                                                     |
| standbySettings           | standby settings when the cluster is a standby cluster                                                                                                                                                                                                                                                                                                                                                                                                                            | if role is "standby"      | StandbySettings   |                                                                                                                                     |
| pgParameters              | a map containing the postgres server parameters and their values. The parameters value don't have to be quoted and single quotes don't have to be doubled since this is already done by the keeper when writing the postgresql.conf file                                                                                                                                                                                                                                          | no                        | map[string]string |                                                                                                                                     |
| pgHBA                     | a list containing additional pg_hba.conf entries. They will be added to the pg_hba.conf generated by stolon. **NOTE**: these lines aren't validated so if some of them are wrong postgres will refuse to start or, on reload, will log a warning and ignore the updated pg_hba.conf file                                                                                                                                                                                          | no                        | []string          | null. Will use the default behiavior of accepting connections from all hosts for all dbs and users with md5 password authentication |

#### ExistingConfig

| Name      | Description                                            | Required | Type   | Default |
|-----------|--------------------------------------------------------|----------|--------|---------|
| keeperUID | the keeperUID to use as the initial master db cluster. | yes      | string |         |


#### NewConfig

| Name          | Description                                                                                                                                                                                                          | Required | Type   | Default |
|---------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|--------|---------|
| locale        | Defines the locale to be used when initializing a new postgres db cluster (initdb `--locale` option). This option isn't validated by stolon so initdb will fail if a wrong option is provided.                       | no       | string |         |
| encoding      | Defines the encoding to be used when initializing a new postgres db cluster (initdb `--encoding` option). This option isn't validated by stolon so initdb will fail if a wrong option is provided.                   | no       | string |         |
| dataChecksums | Defines if data checksums should be enabled when initializing a new postgres db cluster (initdb `--data-checksums` option). This option isn't validated by stolon so initdb will fail if a wrong option is provided. | no       | bool   |         |


#### PITRConfig

| Name                    | Description                                                                                                                                                                                                      | Required | Type                    | Default |
|-------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|-------------------------|---------|
| dataRestoreCommand      | defines the command to execute for restoring the db cluster data. %d is replaced with the full path to the db cluster datadir. Use %% to embed an actual % character. Must return a 0 exit code only on success. | yes      | string                  |         |
| archiveRecoverySettings | archive recovery configuration                                                                                                                                                                                   | yes      | ArchiveRecoverySettings |         |
| recoveryTargetSettings | recovery target configuration                                                                                                                                                                                     | no       | RecoveryTargetSettings  |         |

#### ArchiveRecoverySettings

| Name                    | Description                                                                                                                                                                | Required | Type                    | Default |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|-------------------------|---------|
| restoreCommand          | defines the command to execute for restoring the archives. See the related [postgresql doc](https://www.postgresql.org/docs/current/static/archive-recovery-settings.html) | yes      | string                  |         |

#### RecoveryTargetSettings

These parameters are the same as defined in [postgresql recovery target settings doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html)

| Name                    | Description                                                                                                                                  | Required | Type                    | Default |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|----------|-------------------------|---------|
| recoveryTarget          | See `recovery_target` in the related [postgresql doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html)          | no       | string                  |         |
| recoveryTargetLsn       | See `recovery_target_lsn` in the related [postgresql doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html)      | no       | string                  |         |
| recoveryTargetName      | See `recovery_target_name` in the related [postgresql doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html)     | no       | string                  |         |
| recoveryTargetTime      | See `recovery_target_time` in the related [postgresql doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html)     | no       | string                  |         |
| recoveryTargetXid       | See `recovery_target_xid` in the related [postgresql doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html)      | no       | string                  |         |
| recoveryTargetTimeline  | See `recovery_target_timeline` in the related [postgresql doc](https://www.postgresql.org/docs/current/static/recovery-target-settings.html) | no       | string                  |         |

#### StandbySettings

| Name                    | Description                                                                                                                                                                                                                                                   | Required | Type                    | Default |
|-------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|-------------------------|---------|
| primaryConnInfo         | connection string to connect to the primary server (its value will be placed in the `primary_conninfo` parameter of the instance `recovery.conf` file. See the related [postgresql doc](https://www.postgresql.org/docs/current/static/standby-settings.html) | yes      | string                  |         |
| primarySlotName         | optional replication slot to use (its value will be placed in the `primary_slot_name` parameter of the instance `recovery.conf` file. See the related [postgresql doc](https://www.postgresql.org/docs/current/static/standby-settings.html)                  | no       | string                  |         |
| recoveryMinApplyDelay   | delay recovery for a fixed period of time (its value will be placed in the `recovery_min_apply_delay` parameter of the instance `recovery.conf` file. See the related [postgresql doc](https://www.postgresql.org/docs/current/static/standby-settings.html)  | no       | string                  |         |

#### Special Types
duration types (as described in https://golang.org/pkg/time/#ParseDuration) are signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".


### Cluster Specification management

It's possible to replace the whole current cluster specification or patch only some parts of it (see https://tools.ietf.org/html/rfc7386).
Currently the specification must be provided in json format.


### Cluster Specification patching

``` bash
stolonctl --cluster-name=mycluster update --patch '{ "synchronousReplication" : true }'
```

You can also pass the cluster specification or a patch as a file using the `-f` option:

``` bash
stolonctl --cluster-name=mycluster update --patch -f spec.json
```

Using `-` as the file name means stdin:

``` bash
echo '{ "synchronousReplication" : true }' | stolonctl --cluster-name=mycluster update --patch -f -
```

### Cluster Specification replace

This command will replace the whole cluster specification. The unspecificed options will be populated with their defaults.
``` bash
stolonctl --cluster-name=mycluster update '{ "requestTimeout": "10s", "sleepInterval": "10s" }'
```

## Examples

### Set some postgres parameters:

You can patch the cluster specification providing postgres parameters. For example, if you want to set `log_min_duration_statement = 1s` you can do:

``` bash
stolonctl --cluster-name=mycluster update --patch '{ "pgParameters" : {"log_min_duration_statement" : "1s" } }'
```

### Remove some postgres parameters

To remove a postgres parameter just patch the cluster spec setting the parameter's value to `null`:

``` bash
stolonctl --cluster-name=mycluster update --patch '{ "pgParameters" : {"log_min_duration_statement" : null } }'
```

### Remove all the postgres parameters

To remove all the postgres parameters just patch the cluster spec setting `pgParameters` value to `null`:

``` bash
stolonctl --cluster-name=mycluster update --patch '{ "pgParameters" : null }'
```
