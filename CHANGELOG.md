### v0.6.0

This version introduces various interesting new features (like support for upcoming PostgreSQL 10 and standby cluster) and different bug fixes.

#### New features
* Support for PostgreSQL 10 ([#281](https://github.com/sorintlab/stolon/pull/281))
* Standby cluster (for multi site disaster recovery and near zero downtime migration) ([#283](https://github.com/sorintlab/stolon/pull/283))
* Old dead keeper removal ([#280](https://github.com/sorintlab/stolon/pull/280))
* On asynchronous clusters elect master only if behind a user defined lag ([#268](https://github.com/sorintlab/stolon/pull/268))
* Docker standalone, swarm and compose examples ([#231](https://github.com/sorintlab/stolon/pull/231)) and ([#238](https://github.com/sorintlab/stolon/pull/238))

#### BugFixes

* Fix incorrect parsing of `synchronous_standby_names` when using synchronous replication with two or more synchronous standbys ([#264](https://github.com/sorintlab/stolon/pull/264))
* Fix non atomic writes of local state files ([#265](https://github.com/sorintlab/stolon/pull/265))

and [many other](https://github.com/sorintlab/stolon/milestone/5)

Thanks to everybody who contributed to this release:

Alexander Ermolaev, Dario Nieuwenhuis, Euan Kemp, Ivan Sim, Jasper Siepkes, Niklas Hambüchen, Sajal Kayan


### v0.5.0

This version is a big step forward previous releases and provides many new features and a better cluster management.

* Now the configuration is fully declarative (see [cluster specification](doc/cluster_spec.md) documentation) ([#178](https://github.com/sorintlab/stolon/pull/178)).
* Ability to create a new cluster starting from a previous backup (point in time recovery) ([#183](https://github.com/sorintlab/stolon/pull/183))
 * Wal-e backup/restore example ([#183](https://github.com/sorintlab/stolon/pull/183))
* Better synchronous replication, the user can define a min and a max number of required synchronous standbys and the master will always block waiting for acknowledge by the required sync standbys. Only synchronous standbys will be elected as new master. ([#219](https://github.com/sorintlab/stolon/pull/219))
* Production ready kubernetes examples (just change the persistent volume provider) ([#215](https://github.com/sorintlab/stolon/pull/215))
* To keep an unique managed central configuration, the postgresql parameters can now only be managed only using the cluster specification ([#181](https://github.com/sorintlab/stolon/pull/181))
* When (re)initializing a new cluster (with an empty db, from an existing instance or from a backup) the postgresql parameters are automatically merged in the cluster spec ([#181](https://github.com/sorintlab/stolon/pull/181))
* Use only store based communication and discovery (removed all the kubernetes specific options) ([#195](https://github.com/sorintlab/stolon/pull/195))
* Ability to use TLS communication with the store (for both etcd and consul) ([#208](https://github.com/sorintlab/stolon/pull/208))
* Better standby monitoring and replacement ([#218](https://github.com/sorintlab/stolon/pull/218))
* Improved logging ([#187](https://github.com/sorintlab/stolon/pull/187))

Many other [improvements and bug fixes](https://github.com/sorintlab/stolon/milestone/4)

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
