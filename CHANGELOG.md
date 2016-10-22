### v0.4.0

Some cleanups and changes in preparation for release v0.5.0 that will receive a big refactor (with different breaking changes) needed to bring a lot of new features.

### v0.3.0

* Support multiple stores via [libkv](https://github.com/docker/libkv) ([#102](https://github.com/sorintlab/stolon/pull/102)). Currently etcd and consul are supported. 
* Can use pg_rewind to sync slaves instead of doing a full resync ([#122](https://github.com/sorintlab/stolon/pull/122)).
* The `--initial-cluster-config` option has been added to the `stolon-sentinel` to provide an initial cluster configuration ([#107](https://github.com/sorintlab/stolon/pull/107)).
* A cluster config option for initializing the cluster also if multiple keepers are registred has been added ([#106](https://github.com/sorintlab/stolon/pull/106)). By default a sentinel won't initialize a new if multiple keepers are registered since it cannot know which one should be the master. With this option a random keeper will be choosed as the master. This is useful when an user wants to create a new cluster with an empty database and starting all the keeper together instead of having to start only one keeper, wait it to being elected as master and then starting the other keepers.
* The `--discovery-type` option has been added to the `stolon-sentinel` to choose if keeper discovery should be done using the store or kubernetes ([#129](https://github.com/sorintlab/stolon/pull/129)).
* Various options has been added to the `stolon-keeper` for setting postgres superuser, replication and initial superuser usernames and passwords ([#136](https://github.com/sorintlab/stolon/pull/136)). 
* Numerous enhancements and bugfixes.

Thanks to all the contributors!


### v0.2.0

* A stolon client (stolonctl) is provided. At the moment it can be used to get clusters list, cluster status and get/replace/patch cluster config ([#28](https://github.com/sorintlab/stolon/pull/28) [#64](https://github.com/sorintlab/stolon/pull/64)). In future multiple additional functions will be added. See [doc/stolonctl.md](doc/stolonctl.md).
* The cluster config is now configurable using stolonctl ([#2](https://github.com/sorintlab/stolon/pull/2)). See [doc/cluster_config.md](doc/cluster_config.md).
* Users can directly put their preferred postgres configuration files inside a configuration directory ($dataDir/postgres/conf.d or provided with --pg-conf-dir) (see [doc/postgres_parameters.md](doc/postgres_parameters.md))
* Users can centrally manage global postgres parameters. They can be configured in the cluster configuration (see [doc/postgres_parameters.md](doc/postgres_parameters.md))
* Now the stolon-proxy closes connections on etcd error. This will help load balancing multiple stolon proxies ([#74](https://github.com/sorintlab/stolon/pull/74) [#76](https://github.com/sorintlab/stolon/pull/76) [#80](https://github.com/sorintlab/stolon/pull/80)).
* kubernetes: added readiness probe for stolon proxy ([#82](https://github.com/sorintlab/stolon/pull/82))
* The keeper takes an exclusive fs lock on its datadir ([#48](https://github.com/sorintlab/stolon/pull/48))
* Numerous bug fixes and improved tests.
