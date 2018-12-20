### v0.14.0

#### Upgrades notes.
 * The `stolonctl clusterdata` command has been splitted into two:
    1. `stolonctl clusterdata read` which will be used to read the current clusterdata.
    2. `stolonctl clusterdata write` which will be used to write the new clusterdata into the new store.

### v0.13.0

#### New features

* Add a `stolonctl` command to force fail a keeper ([#546](https://github.com/sorintlab/stolon/pull/546))
* Overcome PostgreSQL synchronous replication limitation that could cause lost transactions under some events ([#514](https://github.com/sorintlab/stolon/pull/514))
* Users can now define `archiveRecoverySettings` in the cluster spec of a standby cluster. One of the possible use cases is to feed the standby cluster only with archived logs without streaming replication. (See Upgrade Notes) ([#543](https://github.com/sorintlab/stolon/pull/543))
* Keeper: remove trailing new lines from provided passwords ([#548](https://github.com/sorintlab/stolon/pull/548))

#### Bug Fixes

* Sort keepers addresses in `pg_hba.conf` to avoid unneeded postgres instance reloads ([#558](https://github.com/sorintlab/stolon/pull/558))
* Set `recovery_target_action` to promote when using recovery target settings [#545](https://github.com/sorintlab/stolon/pull/545))
* Fixed wrong listen address used in `pg_hba.conf` when `SUReplAccessStrict` mode was enabled ([#520](https://github.com/sorintlab/stolon/pull/520))

and [many other](https://github.com/sorintlab/stolon/milestone/12) bug fixes and documentation improvements.

Thanks to everybody who contributed to this release.

#### Upgrades notes.

* The clusterspec `standbySettings` option as been replaced by the `standbyConfig` option. Internally it can contain two fields `standbySettings` and `archiveRecoverySettings` (see the clusterspec doc with the descriptors of this new option). If you're updating a standby cluster, BEFORE starting it you should update, using `stolonctl`, the clusterspec with the new `standbyConfig` option.


### v0.12.0

#### New features

* Detect and report when keeper persistent data dir is not the expected one (usually due to wrong configuration, non persistent storage etc...) ([#510](https://github.com/sorintlab/stolon/pull/510))
* Support PostgresSQL 11 (beta) ([#513](https://github.com/sorintlab/stolon/pull/513))
* Replication slots declared in the clusterspec `additionalMasterReplicationSlots` option will now be prefixed with the `stolon_` string to let users be able to manually create/drop custom replication slots (See Upgrade Notes) ([#531](https://github.com/sorintlab/stolon/pull/531))

#### Bug Fixes

* fix wrong address in pg_hba.conf when clusterspec `defaultSUReplAccessMode` is `strict` ([#520](https://github.com/sorintlab/stolon/pull/520))

and [many other](https://github.com/sorintlab/stolon/milestone/11) bug fixes and documentation improvements.

Thanks to everybody who contributed to this release:

Alexandre Assouad, Lothar Gesslein, @nseyvet

#### Upgrades notes.

* Replication slots declared in the clusterspec `additionalMasterReplicationSlots` option will now be prefixed with the `stolon_` string to let users be able to manually create/drop custom replication slots (they shouldn't start with `stolon_`). Users of these feature should upgrade all the references to these replication slots adding the `stolon_` prefix.

### v0.11.0

#### New features

* In the k8s store backend, stolon components discovery now uses the `component` label instead of the `app` label (See Upgrade Notes) ([#469](https://github.com/sorintlab/stolon/pull/469))
* Improved docker swarm examples to resemble the k8s one ([#482](https://github.com/sorintlab/stolon/pull/482))
* If the user enabled ssl/tls use it also for replication/pg_rewind connections ([#501](https://github.com/sorintlab/stolon/pull/501))
* Remove final newline from example base64 password in k8s example ([#505](https://github.com/sorintlab/stolon/pull/505))

#### Bug Fixes

* Fixed wrong libkv store election path (See Upgrade Notes) ([#479](https://github.com/sorintlab/stolon/pull/479))
* Fixed a check in synchronous replication that will block future synchronous standbys updates under some circumstances ([#494](https://github.com/sorintlab/stolon/pull/494))
* Fixed atomic writes of postgresql genenerated files ([#495](https://github.com/sorintlab/stolon/pull/495))

Thanks to everybody who contributed to this release:

Bill Helgeson, Niklas Hambüchen, Sylvere Richard, Tyler Kellen


#### Upgrades notes.

* In the k8s store backend, the label that defines the kind of stolon component has changed from `app` to `component`. When upgrading you should update the various resource descriptors setting the k8s component name (`stolon-keeper`, `stolon-sentinel`, `stolon-proxy`) inside the `component` label instead of the `app` label.
* When using the etcdv2 store, due to a wrong leader election path introduced in the last release and now fixed, if your sentinel returns an election error like `election loop error {"error": "102: Not a file ...` you should stop all the sentinels and remove the wrong dir using `etcdctl rmdir /stolon/cluster/$STOLONCLUSTER/sentinel-leader` where `$STOLONCLUSTER` should be substituted with the stolon cluster name (remember to set `ETCDCTL_API=2`).

### v0.10.0

#### New features

* Initial support for native kubernetes store ([#433](https://github.com/sorintlab/stolon/pull/433))
* Improved sync standby management ([#444](https://github.com/sorintlab/stolon/pull/444))
* Ability to use strict and dynamic hba entries for keeper replication ([#412](https://github.com/sorintlab/stolon/pull/412))
* Ability to define additional replication slots for external clients ([#434](https://github.com/sorintlab/stolon/pull/434))
* Improved wal level selection ([#450](https://github.com/sorintlab/stolon/pull/450))

Thanks to everybody who contributed to this release:

Pierre Alexandre Assouad, Arun Babu Neelicattu, Sergey Kim

### v0.9.0

#### New features

* The logs will be colored only when on a tty or when `--log-color` is provided ([#416](https://github.com/sorintlab/stolon/pull/416))
* Now the store prefix is configurable `--store-prefix` ([#425](https://github.com/sorintlab/stolon/pull/425))

#### BugFixes

* Fixed keeper missing waits for instance ready ([#418](https://github.com/sorintlab/stolon/pull/418))
* Fixed etcdv3 store wrong get leader timeout causing `stolonctl status` errors ([#426](https://github.com/sorintlab/stolon/pull/426))

Thanks to everybody who contributed to this release:

Pierre Fersing, Dmitry Andreev

### v0.8.0

#### New features

* Added support for etcd v3 api (using --store-backend etcdv3) ([#393](https://github.com/sorintlab/stolon/pull/393))
* Now the stolon-proxy has tcp keepalive enabled by default and provides options for tuning its behavior ([#357](https://github.com/sorintlab/stolon/pull/357))
* Added `removekeeper` command to stolonctl ([#383](https://github.com/sorintlab/stolon/pull/383))
* Added the ability to choose the authentication method for su and replication user (currently one of md5 or trust) ([#380](https://github.com/sorintlab/stolon/pull/380))

#### BugFixes
* Fixed and improved db startup logic to handle a different pg_ctl start behavior between postgres 9 and 10 ([#401](https://github.com/sorintlab/stolon/pull/401))
* Fixed keeper datadir locking ([#405](https://github.com/sorintlab/stolon/pull/405))

and [many other](https://github.com/sorintlab/stolon/milestone/7) bug fixes and documentation improvements.

Thanks to everybody who contributed to this release:

AmberBee, @emded, Pierre Fersing

### v0.7.0

#### New features

* Added ability to define custom pg_hba.conf entries ([#341](https://github.com/sorintlab/stolon/pull/341))
* Added ability to set Locale, Encoding and DataChecksums when initializing a new pg db cluster ([#338](https://github.com/sorintlab/stolon/pull/338))
* Added stolonctl `clusterdata` command to dump the current clusterdata saved in the store ([#318](https://github.com/sorintlab/stolon/pull/318))
* Detect if a standby cannot sync due to missing wal files on primary ([#312](https://github.com/sorintlab/stolon/pull/312))
* Various improvements to proxy logic ([#308](https://github.com/sorintlab/stolon/pull/308)) ([#310](https://github.com/sorintlab/stolon/pull/310))
* Added cluster spec option to define additional wal senders ([#311](https://github.com/sorintlab/stolon/pull/311))
* Added various postgresql recovery target settings for point in time recovery ([#303](https://github.com/sorintlab/stolon/pull/303))
* Added `--log-level` argument to stolon commands (deprecating `--debug`)  ([#298](https://github.com/sorintlab/stolon/pull/298))

#### BugFixes
* IPV6 fixes ([#326](https://github.com/sorintlab/stolon/pull/326))
* Handle null values in pg_file_settings view ([#322](https://github.com/sorintlab/stolon/pull/322))

and [many other](https://github.com/sorintlab/stolon/milestone/6) bug fixes and documentation improvements

Thanks to everybody who contributed to this release:

Albert Vaca, @emded, Niklas Hambüchen, Tim Heckman

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
