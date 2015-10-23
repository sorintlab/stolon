## Cluster Configuration ##

The cluster configuration is saved in the cluster view. It's updatable using [stolonctl](stolonctl.md).

It's possible to replace the whole current configuration or patch only some configuration options.

### Configuration replace example

``` bash
echo '{ "requestTimeout": "10s", "sleepInterval": "10s" }' | stolonctl --cluster-name=mycluster config replace -f - '
```


### Configuration patch example

``` bash
stolonctl --cluster-name=mycluster config patch '{ "synchronousreplication" : true }'
```

or

``` bash
echo '{ "synchronousreplication" : true }' | stolonctl --cluster-name=mycluster config patch -f -
```

In both commands the configuration must be provided in json format.


### Configuration Format.

By default the cluster configuration is empty. For every empty field a default is defined. This is the whole configuration with its defaults in json format:

``` json
{
    "requestTimeout": "10s",
    "sleepInterval": "5s",
    "keeperFailInterval": "20s",
    "pgreplUser": "username",
    "pgreplPassword": "password",
    "maxStandbysPerSender": 3,
    "synchronousReplication": false
}
```


* requestTimeout: (duration) time after which any request (keepers checks from sentinel etc...) will fail.
* sleepInterval: (duration) interval to wait before next check (for every component: keeper, sentinel, proxy).
* keeperFailInterval: (duration) interval after the first fail to declare a keeper as not healthy.
* pgreplUser: (string) PostgreSQL replication username
* pgreplPassword: (string) PostgreSQL replication password
* maxStandbysPerSender: (uint) max number of standbys for every sender. A sender can be a master or another standby (with cascading replication).
* synchronousReplication: (bool) use synchronous replication between the master and its standbys


duration types (as described in https://golang.org/pkg/time/#ParseDuration) are signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
