# Re-sync a broken replica

Due to unforeseen circumstances, if you end up with an empty postgres data directory on one of your replicas but your _master_ is intact, you will receive a message that looks something like this:

```
different local dbUID but init mode is none, this shouldn't happen. Something bad happened to the keeper data. Check that keeper data is on a persistent volume and that the keeper state files weren't removed
```

This is because all keepers _require_ persistent storage and will not auto "re-sync" just because they find an empty data directory while still having an entry in the clusterdata.

In this case, you must use the `removekeeper` subcommand to manually remove the "bad" keeper from the cluster data as well.

```
stolonctl removekeeper postgres2
```

This causes the keeper with uid `postgres2` process to register itself as a "new" one and begin the re-sync process.
Of course, the above assumes that the master is intact and streaming data correctly.
