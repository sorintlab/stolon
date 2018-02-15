## Stolon Client (stolonctl)

`stolonctl` is the stolon client which controls the stolon cluster(s)

It needs to communicate with the cluster store (providing `--store-backend` and related option) on which the requested cluster name (`--cluster-name`) is running.

To avoid repeating for every command (or inside scripts) all the options you can export them as environment variables. Their name will be the same as the option name converted in uppercase, with `_` replacing `-` and prefixed with `STOLONCTL_`.

Ex.
```
STOLONCTL_STORE_BACKEND
STOLONCTL_STORE_ENDPOINTS
STOLONCTL_CLUSTER_NAME
```

### See also

[stolonctl command invocation](commands/stolonctl.md)
