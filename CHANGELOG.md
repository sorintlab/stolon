
### v0.2.0

* A stolon client (stolonctl) is provided. At the moment it can be used to get clusters list, cluster status and get/replace/patch cluster config ([#28](https://github.com/sorintlab/stolon/pull/28) [#64](https://github.com/sorintlab/stolon/pull/64)). In future multiple additional functions will be added. See [doc/stolonctl.md](doc/stolonctl.md).
* The cluster config is now configurable using stolonctl ([#2](https://github.com/sorintlab/stolon/pull/2)). See [doc/cluster_config.md](doc/cluster_config.md).
* Users can directly put their preferred postgres configuration files inside a configuration directory ($dataDir/postgres/conf.d or provided with --pg-conf-dir) (see [doc/postgres_parameters.md](doc/postgres_parameters.md))
* Users can centrally manage global postgres parameters. They can be configured in the cluster configuration (see [doc/postgres_parameters.md](doc/postgres_parameters.md))
* Now the stolon-proxy closes connections on etcd error. This will help load balancing multiple stolon proxies ([#74](https://github.com/sorintlab/stolon/pull/74) [#76](https://github.com/sorintlab/stolon/pull/76) [#80](https://github.com/sorintlab/stolon/pull/80)).
* kubernetes: added readiness probe for stolon proxy ([#82](https://github.com/sorintlab/stolon/pull/82))
* The keeper takes an exclusive fs lock on its datadir ([#48](https://github.com/sorintlab/stolon/pull/48))
* Numerous bug fixes and improved tests.
