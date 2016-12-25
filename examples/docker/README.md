# Dockerized Stolon
Here are some examples on running a Dockerized Stolon cluster.

## Table of Content

* [Docker Images](#docker-images)
* [Local Cluster](#local-cluster)
* [Docker Compose](#docker-compose)

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

## Docker Compose
This example sets up a Docker Compose Stolon cluster with 1 Sentinel instance, 3 Keeper instances, 1 Proxy instance and 3 etcd instances on your local environment. All containers are connected to a user-defined bridge network, named `stolon-network`. It is tested with Docker 1.12.5 and Docker Compose 1.9.0.

To get started, build all the Docker images by running:
```sh
$ make build
```

Then run the `compose-up` target in the Makefile:
```sh
$ export ETCD_TOKEN=<some-token>
$ make compose-up
```
The `docker-compose.yml` file reads all its required variables from the `.env` file.

Once `make compose-up` is completed, make sure all the services are running:
```sh
$ docker-compose ps
      Name                     Command               State                 Ports
-----------------------------------------------------------------------------------------------
etcd-00             etcd --name=etcd-00 --data ...   Up      2379/tcp, 2380/tcp
etcd-01             etcd --name=etcd-01 --data ...   Up      2379/tcp, 2380/tcp
etcd-02             etcd --name=etcd-02 --data ...   Up      2379/tcp, 2380/tcp
keeper-00           stolon-keeper --pg-su-user ...   Up      5432/tcp
keeper-01           stolon-keeper --pg-su-user ...   Up      5432/tcp
keeper-02           stolon-keeper --pg-su-user ...   Up      5432/tcp
stolon_proxy_1      stolon-proxy --store-endpo ...   Up      0.0.0.0:32794->25432/tcp, 5432/tcp
stolon_sentinel_1   stolon-sentinel --store-en ...   Up      5432/tcp
```

To make sure the etcd cluster is running:
```sh
$ docker-compose exec etcd-00 etcdctl cluster-health
member 7883f95c8b8e92b is healthy: got healthy result from http://etcd-00:2379
member 7da9f70288fafe07 is healthy: got healthy result from http://etcd-01:2379
member 93b4b1aeeb764068 is healthy: got healthy result from http://etcd-02:2379
cluster is healthy
```

To scale the number of Keeper instances:
```sh
$ docker-compose scale keeper=3
```

Notice that Sentinel will detect the new Keeper instances:
```sh
$ docker-compose logs -f sentinel
Attaching to stolon_sentinel_1
sentinel_1  | [I] 2016-12-25T06:23:29Z sentinel.go:1009: added new standby db db=22c06996 keeper=56fdf72c
......
sentinel_1  | [I] 2016-12-25T06:23:29Z sentinel.go:1009: added new standby db db=5583be81 keeper=5967f252
sentinel_1  | [W] 2016-12-25T06:23:34Z sentinel.go:245: received db state for unexpected db uid receivedDB= db=22c06996
sentinel_1  | [W] 2016-12-25T06:23:34Z sentinel.go:245: received db state for unexpected db uid receivedDB= db=5583be81
```

To make sure you can connect to the Stolon cluster using the Proxy's published port:
```sh
$ docker-compose port proxy 25432
0.0.0.0:32794

$ psql -h 127.0.0.1 -p 32794 -U postgres
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

To make sure that the replication is working correctly, you will have to stop the master Keeper container using `docker stop` since Docker Compose commands only work with Compose services. Note that scaling down the Keeper services using `docker-compose scale` won't work because you won't have control over which replicas to stop. The `docker-compose logs -f sentinel` command can be used to determine the master keeper's ID, and monitor the failover process.

For example,
```sh
$ docker-compose logs -f sentinel
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
$ docker stop stolon_keeper_1 # your master keeper might be different
$ docker-compose logs -f sentinel
sentinel_1  | [I] 2016-12-25T06:40:47Z sentinel.go:743: master db is failed db=0d80aba2 keeper=c7280058
sentinel_1  | [I] 2016-12-25T06:40:47Z sentinel.go:754: trying to find a new master to replace failed master
sentinel_1  | [I] 2016-12-25T06:40:47Z sentinel.go:785: electing db as the new master db=8b4e0342 keeper=1bfb47ae
sentinel_1  | [E] 2016-12-25T06:40:52Z sentinel.go:234: no keeper info available db=0d80aba2 keeper=c7280058
sentinel_1  | [I] 2016-12-25T06:40:52Z sentinel.go:844: removing old master db db=0d80aba2
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

To destroy the entire cluster, run `make compose-clean` to stop and remove all etcd and stolon containers. By default, the Keepers' volumes aren't removed. To purge the containers and their respective volumes, use `make compose-purge`.

### Known Issues
The Sentinel seems unable to detect removed Keeper instances when the service is scaled down using `docker-compose scale`. To reproduce,
1. Scale up the Keeper service with `docker-compose scale keeper=3`.
1. Scale down the service with `docker-compose scale keeper=1`.
1. The Sentinel logs show that Sentinel continues to probe the removed replicas.
