# Upgrading from v0.4.0 to v0.5.0

Stolon v0.5.0 received a big rework to improve its internal data model and implement new features. To upgrade an existing cluster from v0.4.0 to v0.5.0 you can follow these steps.

* Annotate the master keeperUID (previously called keeper id). You can retrieve this using `stolonctl status`
* Stop all the cluster processes (keepers, sentinels and proxies)
* Upgrade the binaries to stolon v0.5.0
* Relaunch all the cluster processes. They will loop reporting `unsupported clusterdata format version 0`.
* Initialize a new cluster data using the master keeperUID:
```
stolonctl init '{ "initMode": "existing", "existingConfig": { "keeperUID": "keeper01" } }'
```

