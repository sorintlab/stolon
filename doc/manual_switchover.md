# Manual switch master without losing any transaction

If for any reason (eg. maintenance) you want to switch the current master to another one without losing any transaction you can do this in these ways:

* If you've synchronous replication enabled you can just stop the current master keeper, one of the synchronous standbys will be elected as the new master.

* If you aren't using synchronous replication you can just temporarily enable it (see [here](syncrepl.md)), wait that the cluster reconfigures some synchronous standbys (you can monitor `pg_stat_replication` for a standby with `sync_state` = `sync`) and then stop the master keeper, wait for a new synchronous standby to be elected and disable synchronous replication.
