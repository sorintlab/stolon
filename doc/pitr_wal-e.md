## Point in time recovery with wal-e

This example shows how to do point in time recovery with stolon using [wal-e](https://github.com/wal-e/wal-e)

[wal-e](https://github.com/wal-e/wal-e) correctly suggests to not put environment variables containing secret data (like aws secret keys) inside the `archive_command` since every user connected to postgres could read them. In its examples wal-e suggests to use the `envdir` command to set the wal-e required environment variables or (since some distribution don't have it) just use a custom script that sets them.

### Backups

#### Base backups

Take the base backups using the `wal-e backup-push` command.

#### Archive backups

For doing this you should set at least the `archive_mode` and the `archive_command` pgParameters in the cluster spec. Wal-e will be used as the archive command:

```
stolonctl update --patch '{ "pgParameters" : { "archive_mode": "on", "archive_command": "envdir /etc/wal-e.d/env wal-e wal-push %p" } }'
```


### Execute a point in time recovery

```
stolonctl init '{ "initMode": "pitr", "pitrConfig": { "dataRestoreCommand": "envdir /etc/wal-e.d/env wal-e backup-fetch %d LATEST" , "archiveRecoverySettings": { "restoreCommand": "envdir /etc/wal-e.d/env wal-e wal-fetch \"%f\" \"%p\"" } } }'
```

