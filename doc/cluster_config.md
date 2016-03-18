## Cluster Configuration ##

The cluster configuration is saved in the cluster view. It's updatable using [stolonctl](stolonctl.md).

It's possible to replace the whole current configuration or patch only some configuration options (see https://tools.ietf.org/html/rfc7386).

### Configuration replace example

``` bash
echo '{ "request_timeout": "10s", "sleep_interval": "10s" }' | stolonctl --cluster-name=mycluster config replace -f - '
```


### Configuration patch example

``` bash
stolonctl --cluster-name=mycluster config patch '{ "synchronous_replication" : true }'
```

or

``` bash
echo '{ "synchronous_replication" : true }' | stolonctl --cluster-name=mycluster config patch -f -
```

In both commands the configuration must be provided in json format.


### Configuration Format.

By default the cluster configuration is empty. For every empty field a default is defined. This is the whole configuration with its defaults in json format:

``` json
{
    "request_timeout": "10s",
    "sleep_interval": "5s",
    "keeper_fail_interval": "20s",
    "pg_repl_user": "username",
    "pg_repl_password": "password",
    "max_standbys_per_sender": 3,
    "synchronous_replication": false,
    "init_with_multiple_keepers": false,
    "use_pg_rewind": false,
    "pg_parameters": null
}
```


* request_timeout: (duration) time after which any request (keepers checks from sentinel etc...) will fail.
* sleep_interval: (duration) interval to wait before next check (for every component: keeper, sentinel, proxy).
* keeper_fail_interval: (duration) interval after the first fail to declare a keeper as not healthy.
* pg_repl_user: (string) PostgreSQL replication username
* pg_repl_password: (string) PostgreSQL replication password
* max_standbys_per_sender: (uint) max number of standbys for every sender. A sender can be a master or another standby (with cascading replication).
* synchronous_replication: (bool) use synchronous replication between the master and its standbys
* init_with_multiple_keepers: (bool) Choose a random initial master when multiple keeper are registered. Used only at cluster initialization (empty clusterview).
* use_pg_rewind: (bool) try to use pg_rewind for faster instance resyncronization.
* pg_parameters: (map[string]string) a map containing the postgres server parameters and their values.


duration types (as described in https://golang.org/pkg/time/#ParseDuration) are signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".


### Setting postgres parameters:

You can patch the cluster config setting the needed postgres parameters. For example, if you want to set `log_min_duration_statement = 1s` you can do:

```
stolonctl --cluster-name=mycluster config patch '{ "pg_parameters" : {"log_min_duration_statement" : "1s" } }'
```

### Removing some postgres parameters

To remove a postgres parameter just patch the config setting the parameter's value to `null`:

```
stolonctl --cluster-name=mycluster config patch '{ "pg_parameters" : {"log_min_duration_statement" : null } }'
```

### Removing all the postgres parameters

To remove all the postgres parameters just patch the config setting `pg_parameters` value to `null`:

```
stolonctl --cluster-name=mycluster config patch '{ "pg_parameters" : null }'
```
