## Cluster initialization

The cluster initialization can be done at the first time when the cluster isn't initiliazed (empty cluster data in the store) or also on an already initialized cluster, dropping it and creating a new one. In this case be careful that the current cluster data will be overwritten and, depending on how you are specifying the cluster specification, the keeper may erase/overwrite their managed postgreg db cluster.

### Initialize a new stolon cluster with a new postgres db cluster

You can initialize new stolon cluster with a new postgres db cluster using
```
stolonctl init
```

This is the same as passing a cluster specification with `initMode` set to `new`:

```
stolonctl init '{ "initMode": "new" }'
```

### Initialize a new stolon cluster using an existing keeper

This can be useful if you want, for whatever reason, to force a new master (with all the possible problems caused by doing this).

Given the declarative nature of the cluster specification you cannot force a new master. So, if you have an existing keeper that you want to set as the new master, you have to initialize a new cluster asking that it should be initialized with a specified keeper as the initial master:


```
stolonctl init '{ "initMode": "existing", "existingConfig": { "keeperUID": "keeper01" } }'
```

### First time initialization without stolonctl

You can also provide the `--initial-cluster-spec` option to the `stolon-sentinel` but this will work only when the clusterdata in the store is empty. 

