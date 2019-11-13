## Point in time recovery

Stolon can do a point a time recovery starting from an existing backup.

* [This](pitr_wal-g.md) example shows how to do point in time recovery with stolon using [wal-g](https://github.com/wal-g/wal-g)
* [This](pitr_wal-e.md) example shows how to do point in time recovery with stolon using [wal-e](https://github.com/wal-e/wal-e)


### Backups

#### Base backups

stolon doesn't trigger base backups, you can run them at your preferred times and with your preferred scheduler.

#### Archive backups

With stolon you should instead enable `archive_mode` and set the `archive_command`, there's nothing different than a typical [postgresql backup](https://www.postgresql.org/docs/current/static/continuous-archiving.html).

```
stolonctl update --patch '{ "pgParameters" : { "archive_mode": "on", "archive_command": "/path/to/your/archive/command %p" } }'
```

`archive_mode` and the related `archive_command` will be enabled for all the instances (master and standbys). This is done to avoid losing some wals to backup when the current master keeper is down and a new master is elected. We suggest to define your archive command script to avoid backing up the same wal from all the instances (for example doing this only when the instance is the stolon master and just removing the wal when the instance is a stolon standby).

### Execute a point in time recovery

To trigger a point in time recovery the cluster has to be (re)initialized using the `pitr` initMode in the cluster specification and setting in the `pitrConfig` the `dataRestoreCommand` and the archive `restoreCommand`.

The `dataRestoreCommand` should containt the local shell command to execute to restore the full backup. Any `%d` in the string is replaced by the full path to the data directory. The command should return a 0 exit status if (and only if) it succeeds.

The archive `restoreCommand` will be used as is in the generated `recovery.conf` file. For its value see the related [postgresql doc](https://www.postgresql.org/docs/current/static/archive-recovery-settings.html)

```
stolonctl init '{ "initMode": "pitr", "pitrConfig": { "dataRestoreCommand": "/path/to/your/backup/restore/command \"%d\"" , "archiveRecoverySettings": { "restoreCommand": "/path/to/your/archive/restore/command \"%f\" \"%p\"" } } }'
```

Note: the `\"` is needed by json to put double quotes inside strings. We aren't using single quotes since they are just used to pass to the shell the full json as a single argument.

When initializing a cluster in pitr init mode a random registered keeper will be choosed and it'll start restoring the database with these steps:

* Remove the current data directory
* Call the `dataRestoreCommand` expanding every %d to the data directory full path. If it exits with a non zero exit code then stop here since something went wrong.
* Create a `recovery.conf` with the right parameters and with `restore_command` set to `restoreCommand`.
* Start the postgres instance and wait for the archive recovery.


If something goes wrong you can see the errors in the keeper's logs or in postgresql log (if these are related to the archive restore step) and you can retrigger a new pitr reinitializing the cluster.
