# Dockerized Stolon
Here are some examples on running a Dockerized Stolon cluster.

## Table of Content

* [Docker Images](#docker-images)
* [Local Cluster](#local-cluster)

## Docker Images
The following Dockerfiles can be used to build the Docker images of Stolon Sentinel, Keeper and Proxy:
1. Dockerfile-Sentinel
1. Dockerfile-Keeper
1. Dockerfile-Proxy

The `etc/init-spec.json` file provides a minimal default Stolon cluster specification. This file can be modified to change the cluster's initial specification. To build the Keeper images, two secret files must be provided at `etc/secrets/pgsql` and `etc/secrets/pgsql-repl`. The content of the `secrets` folder is git-ignored.

A convenient `build` target is provided in the Makefile to build all the components' image and generate the needed secrets.

## Local Cluster
This example sets up a Stolon cluster of 1 Sentinel instance, 3 Keeper instances, 1 Proxy instance and 3 etcd instances in your local environment. All containers are connected to a user-defined bridge network, named `stolon-network`. It is tested with Docker 1.12.5.

To get started, build the Docker images with:
```sh
$ make build
```

To set up the local cluster, run:
```sh
$ ETCD_TOKEN=<some_token> make local-up
```
The Proxy's port is mapped to a random higher range host port, which can be obtained using the `docker port stolon-proxy` command.

To make sure the etcd cluster is running correctly:
```sh
$ docker exec etcd-00 etcdctl cluster-health
member 7883f95c8b8e92b is healthy: got healthy result from http://etcd-00:2379
member 7da9f70288fafe07 is healthy: got healthy result from http://etcd-01:2379
member 93b4b1aeeb764068 is healthy: got healthy result from http://etcd-02:2379
cluster is healthy
```

To make sure you can connect to the stolon cluster:
```sh
$ docker port stolon-proxy
25432/tcp -> 0.0.0.0:32786

$ psql -h 127.0.0.1 -p 32786 -U postgres
Password for user postgres: # can be obtained from your local secrets/pgsql
psql (9.6.1)
Type "help" for help.

postgres=#
```

Now you can run some SQL query tests against the cluster:
```sh
postgres=# CREATE TABLE test (id INT PRIMARY KEY NOT NULL, value TEXT NOT NULL);
CREATE TABLE
postgres=# INSERT INTO test VALUES (1, 'value1');
INSERT 0 1
postgres=# SELECT * FROM test;
 id | value
----+--------
  1 | value1
(1 row)
```

To make sure that the replication is working correctly, use the `docker stop` command to kill the master keeper. The `docker logs stolon-sentinel` command can be used to determine the master keeper's ID, and monitor the failover process.

For example,
```sh
$ docker logs stolon-sentinel
[I] 2016-12-20T02:16:26Z sentinel.go:1408: sentinel uid uid=0bf65dce
[I] 2016-12-20T02:16:26Z sentinel.go:84: Trying to acquire sentinels leadership
[I] 2016-12-20T02:16:26Z sentinel.go:1310: writing initial cluster data
[I] 2016-12-20T02:16:26Z sentinel.go:91: sentinel leadership acquired
[I] 2016-12-20T02:16:31Z sentinel.go:571: trying to find initial master
[E] 2016-12-20T02:16:31Z sentinel.go:1361: failed to update cluster data error=cannot choose initial master: no keepers registered
[I] 2016-12-20T02:16:36Z sentinel.go:571: trying to find initial master
[I] 2016-12-20T02:16:36Z sentinel.go:576: initializing cluster keeper=db0f03a1
[W] 2016-12-20T02:16:41Z sentinel.go:245: received db state for unexpected db uid receivedDB= db=77c1631c
[I] 2016-12-20T02:16:41Z sentinel.go:614: waiting for db db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:16:46Z sentinel.go:601: db initialized db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:17:01Z sentinel.go:1009: added new standby db db=0d640820 keeper=db402496
[I] 2016-12-20T02:17:01Z sentinel.go:1009: added new standby db db=38d5f2f3 keeper=5001cc2c
.....
$ docker stop stolon-keeper-00 # your master keeper might be different
$ docker logs -f stolon-sentinel
[E] 2016-12-20T02:59:36Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[E] 2016-12-20T02:59:41Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:59:41Z sentinel.go:743: master db is failed db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:59:41Z sentinel.go:754: trying to find a new master to replace failed master
[I] 2016-12-20T02:59:41Z sentinel.go:785: electing db as the new master db=0d640820 keeper=db402496
[E] 2016-12-20T02:59:46Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[E] 2016-12-20T02:59:51Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:59:51Z sentinel.go:844: removing old master db db=77c1631c
```

Once the failover process is completed, you will be able to resume your `psql` session.
```sh
postgres=# SELECT * FROM test;
server closed the connection unexpectedly
        This probably means the server terminated abnormally
        before or while processing the request.
The connection to the server was lost. Attempting reset: Succeeded.
postgres=# SELECT * FROM test;
 id | value
----+--------
  1 | value1
(1 row)
```

To destroy the entire cluster, run `make local-clean` to stop and remove all etcd and stolon containers. By default, the Keepers' volumes aren't removed. To purge the containers and their respective volumes, use `make local-purge`.
