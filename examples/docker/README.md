# Dockerized Stolon
Here are some examples on running a Dockerized Stolon cluster.

All examples are tested with Docker 1.12.5, Docker Compose 1.9.0 and Stolon 0.5.0.

## Table of Content

* [Docker Images](#docker-images)
* [Local Cluster](#local-cluster)
* [Docker Compose](#docker-compose)
* [Docker Swarm](#docker-swarm)

## Docker Images
The following Dockerfiles can be used to build the Docker images of Stolon Sentinel, Keeper and Proxy:

1. Dockerfile-Sentinel
1. Dockerfile-Keeper
1. Dockerfile-Proxy

The `etc/init-spec.json` file provides a minimal default Stolon cluster specification. This file can be modified to change the cluster's initial specification. To build the Keeper images, two secret files must be provided at `etc/secrets/pgsql` and `etc/secrets/pgsql-repl`. The content of the `secrets` folder is git-ignored.

A convenient `build` target is provided in the Makefile to build all the components' image and generate the needed secrets.

## Local Cluster
This example sets up a Stolon cluster of 1 Sentinel instance, 3 Keeper instances, 1 Proxy instance and 3 etcd instances in your local environment. All containers are connected to a user-defined bridge network, named `stolon-network`.

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
....
[I] 2016-12-20T02:16:26Z sentinel.go:1408: sentinel uid uid=0bf65dce
[I] 2016-12-20T02:16:26Z sentinel.go:84: Trying to acquire sentinels leadership
[I] 2016-12-20T02:16:26Z sentinel.go:1310: writing initial cluster data
[I] 2016-12-20T02:16:26Z sentinel.go:91: sentinel leadership acquired
[I] 2016-12-20T02:16:31Z sentinel.go:571: trying to find initial master
[E] 2016-12-20T02:16:31Z sentinel.go:1361: failed to update cluster data error=cannot choose initial master: no keepers registered
[I] 2016-12-20T02:16:36Z sentinel.go:571: trying to find initial master
[I] 2016-12-20T02:16:36Z sentinel.go:576: initializing cluster keeper=db0f03a1 # <--- this is the master keeper
[W] 2016-12-20T02:16:41Z sentinel.go:245: received db state for unexpected db uid receivedDB= db=77c1631c
[I] 2016-12-20T02:16:41Z sentinel.go:614: waiting for db db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:16:46Z sentinel.go:601: db initialized db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:17:01Z sentinel.go:1009: added new standby db db=0d640820 keeper=db402496
[I] 2016-12-20T02:17:01Z sentinel.go:1009: added new standby db db=38d5f2f3 keeper=5001cc2c
.....
$ docker stop stolon-keeper-00 # your master keeper might be different
$ docker logs -f stolon-sentinel
.....
[E] 2016-12-20T02:59:36Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[E] 2016-12-20T02:59:41Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:59:41Z sentinel.go:743: master db is failed db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:59:41Z sentinel.go:754: trying to find a new master to replace failed master
[I] 2016-12-20T02:59:41Z sentinel.go:785: electing db as the new master db=0d640820 keeper=db402496
[E] 2016-12-20T02:59:46Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[E] 2016-12-20T02:59:51Z sentinel.go:234: no keeper info available db=77c1631c keeper=db0f03a1
[I] 2016-12-20T02:59:51Z sentinel.go:844: removing old master db db=77c1631c
.....
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
This example sets up a Docker Compose Stolon cluster with 1 Sentinel instance, 3 Keeper instances, 1 Proxy instance and 3 etcd instances on your local environment. All containers are connected to a user-defined bridge network, named `stolon-network`.

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
......
sentinel_1  | [I] 2016-12-25T06:23:29Z sentinel.go:1009: added new standby db db=22c06996 keeper=56fdf72c
sentinel_1  | [I] 2016-12-25T06:23:29Z sentinel.go:1009: added new standby db db=5583be81 keeper=5967f252
......
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
.....
[I] 2016-12-20T02:16:36Z sentinel.go:576: initializing cluster keeper=db0f03a1 # <--- this is the master keeper
.....
$ docker stop stolon_keeper_1 # your master keeper might be different
$ docker-compose logs -f sentinel
.....
sentinel_1  | [I] 2016-12-25T06:40:47Z sentinel.go:743: master db is failed db=0d80aba2 keeper=c7280058
sentinel_1  | [I] 2016-12-25T06:40:47Z sentinel.go:754: trying to find a new master to replace failed master
sentinel_1  | [I] 2016-12-25T06:40:47Z sentinel.go:785: electing db as the new master db=8b4e0342 keeper=1bfb47ae
sentinel_1  | [E] 2016-12-25T06:40:52Z sentinel.go:234: no keeper info available db=0d80aba2 keeper=c7280058
sentinel_1  | [I] 2016-12-25T06:40:52Z sentinel.go:844: removing old master db db=0d80aba2
.....
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

## Docker Swarm
This example sets up a Stolon cluster with Docker swarm on either your local environment with Virtualbox + Boot2Docker or DigitalOcean. It uses Docker Machine to set up 1 Swarm Manager and 3 Swarm workers. The Stolon cluster is made up of 1 Sentinel instance, 3 Keeper instances, 1 Proxy instance and 3 etc instances. Note that the droplets created on DigitalOcean aren't free.

To get started, set up the VMs using either:
```sh
$ make swarm-local
```
or
```sh
$ DO_ACCESS_TOKEN=<your-access-token> DO_SSH_KEY_FINGERPRINT=<fingerprint> make swarm-droplets
```
The DigitalOcean access token and SSH key fingerprint can be generated and obtained from the DigitalOcean web console.

To confirm that the VMs are running:
```sh
$ docker-machine ls
NAME              ACTIVE   DRIVER         STATE     URL                          SWARM   DOCKER    ERRORS
swarm-manager     -        digitalocean   Running   tcp://xxx.xxx.xxx.xxx:2376           v1.12.5
swarm-worker-00   -        digitalocean   Running   tcp://xxx.xxx.xxx.xxx:2376           v1.12.5
swarm-worker-01   -        digitalocean   Running   tcp://xxx.xxx.xxx.xxx:2376           v1.12.5
swarm-worker-02   -        digitalocean   Running   tcp://xxx.xxx.xxx.xxx:2376           v1.12.5
```

Then you can initialize Docker Swarm with:
```sh
$ make swarm-init
```
This configures the `swarm-manager` VM to be the Swarm Leader Manager with `swarm-worker-00`, `swarm-worker-01` and `swarm-worker-02` serving as the Swarm Workers.

To confirm that the Swarm cluster is configured correctly:
```sh
$ eval `docker-machine env swarm-manager`
$ docker node ls
ID                           HOSTNAME         STATUS  AVAILABILITY  MANAGER STATUS
6ya2x5bmdear0gsgibule19uv    swarm-worker-02  Ready   Active
8m2ugdyw0rb7dnum9xgeratbi *  swarm-manager    Ready   Drain         Leader
9g7vaofjs8eysuax95mghn9sb    swarm-worker-01  Ready   Active
f55xnxwkjeujjlo5cgcq7uq14    swarm-worker-00  Ready   Active
```
The Swarm Manager is configured to have `Drain` availability implying that no service tasks (i.e. containers) will be scheduled to run on it.

Once all the nodes (i.e. VMs) are live, the Stolon component images must be built on all VMs before any Stolon containers can be run:
```sh
make swarm-build
```
This target runs the `docker build` command on every node in the Swarm cluster. It will take a few minutes to complete the build.

To confirm that the build completed successfully:
```sh
$ eval `docker-machine env swarm-manager`
$ docker images
REPOSITORY                  TAG                 IMAGE ID            CREATED             SIZE
sorintlab/stolon-proxy      0.5.0               9edc0d485664        4 hours ago         301 MB
sorintlab/stolon-keeper     0.5.0               9008fdcb765d        4 hours ago         290.1 MB
sorintlab/stolon-sentinel   0.5.0               e7ff51216416        4 hours ago         301.3 MB
postgres                    9.6.1               0e24dd8079dc        11 days ago         264.9 MB

$ eval `docker-machine env swarm-worker-00`
$ docker images
REPOSITORY                  TAG                 IMAGE ID            CREATED             SIZE
sorintlab/stolon-proxy      0.5.0               vsdd5654d854        4 hours ago         301 MB
sorintlab/stolon-keeper     0.5.0               9d4th77dfbsw        4 hours ago         290.1 MB
sorintlab/stolon-sentinel   0.5.0               u8d9cxjs0psf        4 hours ago         301.3 MB
postgres                    9.6.1               08s7sdfmvaws        11 days ago         264.9 MB

$ eval `docker-machine env swarm-worker-01`
$ docker images
REPOSITORY                  TAG                 IMAGE ID            CREATED             SIZE
sorintlab/stolon-proxy      0.5.0               9edc0d485664        4 hours ago         301 MB
sorintlab/stolon-keeper     0.5.0               9008fdcb765d        4 hours ago         290.1 MB
sorintlab/stolon-sentinel   0.5.0               e7ff51216416        4 hours ago         301.3 MB
postgres                    9.6.1               0e24dd8079dc        11 days ago         264.9 MB

$ eval `docker-machine env swarm-worker-02`
$ docker images
REPOSITORY                  TAG                 IMAGE ID            CREATED             SIZE
sorintlab/stolon-proxy      0.5.0               a1bc2beb28b7        4 hours ago         301 MB
sorintlab/stolon-keeper     0.5.0               a50fb056a39a        4 hours ago         290.1 MB
sorintlab/stolon-sentinel   0.5.0               4bdf9eb2b345        4 hours ago         301.3 MB
postgres                    9.6.1
```

Once the images are built, you can create the Stolon cluster with:
```sh
$ eval `docker-machine env swarm-manager`
$ ETCD_TOKEN=<some-token> make swarm-stolon
```

To confirm that all the services are running:
```sh
$ eval `docker-machine env swarm-manager`
$ docker service ls
ID            NAME      REPLICAS  IMAGE                            COMMAND
2ez3ztkbp26l  etcd-02   1/1       quay.io/coreos/etcd:v3.0.15      /usr/local/bin/etcd --name=etcd-02 --data-dir=data.etcd --advertise-client-urls=http://etcd-02:2379 --listen-client-urls=http://0.0.0.0:2379 --initial-advertise-peer-urls=http://etcd-02:2380 --listen-peer-urls=http://0.0.0.0:2380 --initial-cluster=,etcd-00=http://etcd-00:2380,etcd-01=http://etcd-01:2380,etcd-02=http://etcd-02:2380 --initial-cluster-state=new --initial-cluster-token=xxxxxx
2qywfq79c0c4  etcd-00   1/1       quay.io/coreos/etcd:v3.0.15      /usr/local/bin/etcd --name=etcd-00 --data-dir=data.etcd --advertise-client-urls=http://etcd-00:2379 --listen-client-urls=http://0.0.0.0:2379 --initial-advertise-peer-urls=http://etcd-00:2380 --listen-peer-urls=http://0.0.0.0:2380 --initial-cluster=,etcd-00=http://etcd-00:2380,etcd-01=http://etcd-01:2380,etcd-02=http://etcd-02:2380 --initial-cluster-state=new --initial-cluster-token=xxxxxx
3cwu5iecxpgn  etcd-01   1/1       quay.io/coreos/etcd:v3.0.15      /usr/local/bin/etcd --name=etcd-01 --data-dir=data.etcd --advertise-client-urls=http://etcd-01:2379 --listen-client-urls=http://0.0.0.0:2379 --initial-advertise-peer-urls=http://etcd-01:2380 --listen-peer-urls=http://0.0.0.0:2380 --initial-cluster=,etcd-00=http://etcd-00:2380,etcd-01=http://etcd-01:2380,etcd-02=http://etcd-02:2380 --initial-cluster-state=new --initial-cluster-token=xxxxxx
48usxupkqvc8  keeper    3/3       sorintlab/stolon-keeper:0.5.0
5c5h3e2i787u  sentinel  1/1       sorintlab/stolon-sentinel:0.5.0
ay815zsum0xd  proxy     1/1       sorintlab/stolon-proxy:0.5.0

$ docker service ps etcd-00
ID                         NAME       IMAGE                        NODE             DESIRED STATE  CURRENT STATE          ERROR
2srlhftr4wp7b5tmu9gsjn70x  etcd-00.1  quay.io/coreos/etcd:v3.0.15  swarm-worker-01  Running        Running 2 minutes ago

$ docker service ps etcd-01
ID                         NAME       IMAGE                        NODE             DESIRED STATE  CURRENT STATE          ERROR
30lerzeaatfnbhfycqwk2engb  etcd-01.1  quay.io/coreos/etcd:v3.0.15  swarm-worker-00  Running        Running 2 minutes ago

$ docker service ps etcd-02
ID                         NAME       IMAGE                        NODE             DESIRED STATE  CURRENT STATE          ERROR
7kgaxnjsmz534baxxvjp32t0c  etcd-02.1  quay.io/coreos/etcd:v3.0.15  swarm-worker-02  Running        Running 2 minutes ago

$ docker service ps sentinel
ID                         NAME        IMAGE                            NODE             DESIRED STATE  CURRENT STATE          ERROR
95r09ncjyzgmdx501ven7dfnq  sentinel.1  sorintlab/stolon-sentinel:0.5.0  swarm-worker-01  Running        Running 2 minutes ago

$ docker service ps keeper
ID                         NAME      IMAGE                          NODE             DESIRED STATE  CURRENT STATE          ERROR
dxffdkfs36op4ty09h65pid2f  keeper.1  sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Running        Running 2 minutes ago
6tv7ohrcfvmb0k2d5li99nw9l  keeper.2  sorintlab/stolon-keeper:0.5.0  swarm-worker-02  Running        Running 2 minutes ago
0590gdetd56bx4prritou1tn2  keeper.3  sorintlab/stolon-keeper:0.5.0  swarm-worker-00  Running        Running 2 minutes ago

$ docker service ps proxy
ID                         NAME     IMAGE                         NODE             DESIRED STATE  CURRENT STATE          ERROR
cgc914ycdf5pbd4sdinr2wpmt  proxy.1  sorintlab/stolon-proxy:0.5.0  swarm-worker-00  Running        Running 2 minutes ago
```

To make sure the etcd cluster is healthy, determine the node that any of the etcd instance is scheduled on using `docker service ps`, and run `docker exec` against that node. For example:
```sh
# determine which node etcd-00 is on
$ docker service ps etcd-00
ID                         NAME       IMAGE                        NODE             DESIRED STATE  CURRENT STATE               ERROR
2srlhftr4wp7b5tmu9gsjn70x  etcd-00.1  quay.io/coreos/etcd:v3.0.15  swarm-worker-01  Running        Running 2 minutes ago

# look for its container on swarm-worker-01
$ eval `docker-machine env swarm-worker-01`
$ docker ps
CONTAINER ID        IMAGE                             COMMAND                  CREATED             STATUS              PORTS               NAMES
1493ebf68c0f        quay.io/coreos/etcd:v3.0.15       "/usr/local/bin/etcd "   5 minutes ago       Up 5 minutes        2379-2380/tcp       etcd-00.1.2srlhftr4wp7b5tmu9gsjn70x

# use etcdctl to check cluster health
$ docker exec etcd-00.1.2srlhftr4wp7b5tmu9gsjn70x etcdctl cluster-health
member 7883f95c8b8e92b is healthy: got healthy result from http://etcd-00:2379
member 7da9f70288fafe07 is healthy: got healthy result from http://etcd-01:2379
member 93b4b1aeeb764068 is healthy: got healthy result from http://etcd-02:2379
cluster is healthy
```

To scale the number of Keeper instances:
```sh
$ eval `docker-machine env swarm-manager`
$ docker service scale keeper=4
$ docker service ps keeper
ID                         NAME          IMAGE                          NODE             DESIRED STATE  CURRENT STATE               ERROR
27v9yj0h1cwdojjaty05scwfk  keeper.1      sorintlab/stolon-keeper:0.5.0  swarm-worker-00  Running        Running 5 minutes ago
8wji81wrsrflfkbs59ne8m9n7   \_ keeper.1  sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Shutdown       Shutdown 5 minutes ago
azt0qs5dl2rtnoyqr36zpfdbl  keeper.2      sorintlab/stolon-keeper:0.5.0  swarm-worker-00  Running        Running 5 minutes ago
6pcxbnuwcesdhws3dz7k8poo8   \_ keeper.2  sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Shutdown       Shutdown 5 minutes ago
2g331ue8bgdqtxb02xbsw3wgk   \_ keeper.2  sorintlab/stolon-keeper:0.5.0  swarm-worker-00  Shutdown       Shutdown 5 minutes ago
1osn0uj153vvkw6xpxbp4x858  keeper.3      sorintlab/stolon-keeper:0.5.0  swarm-worker-00  Running        Running 5 minutes ago
1y05i7ilhbnwmgods78nfyk28   \_ keeper.3  sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Shutdown       Shutdown 5 minutes ago
du8t6w8c75rhdjidtnw8e2186  keeper.4      sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Running        Running about a minute ago
6k7skt591htszhc7qxmr9izf0  keeper.5      sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Running        Running about a minute ago
1c62gdxih05lkgcjs8g8oc0bc  keeper.6      sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Running        Running about a minute ago
```
Check the Sentinel's logs to see the new Keeper joins the cluster.

To make sure that you can access the Stolon cluster,
```sh
$ docker service inspect --pretty proxy
....
Ports:
 Protocol = tcp
 TargetPort = 25432
 PublishedPort = 30000
$ psql -U postgres -h <swarm-manager-ip> -p 30000
Password for user postgres: # can be obtained from your etc/secrets/pgsql
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

To make sure that the replication is working correctly, stop the master Keeper container using `docker stop`. The `docker logs -f sentinel` command can be used to determine the master keeper's ID, and monitor the failover process.

For example,
```sh
# find the node the Sentinel is on
$ eval `docker-machine env swarm-manager`
$ docker service ps sentinel
ID                         NAME        IMAGE                            NODE             DESIRED STATE  CURRENT STATE           ERROR
95r09ncjyzgmdx501ven7dfnq  sentinel.1  sorintlab/stolon-sentinel:0.5.0  swarm-worker-01  Running        Running 11 minutes ago

# use the Sentinel's log to determine the master Keeper
$ eval `docker-machine env swarm-worker-01`
$ docker logs -f sentinel.1.95r09ncjyzgmdx501ven7dfnq
.....
[I] 2016-12-26T04:20:44Z sentinel.go:576: initializing cluster keeper=76a713c4  # <----- This is the master keeper
.....

# assume that the master Keeper is container keeper.1.dxffdkfs36op4ty09h65pid2f on swarm-worker-02...
$ eval `docker-machine env swarm-worker-02`
$ docker service ps keeper
ID                         NAME      IMAGE                          NODE             DESIRED STATE  CURRENT STATE           ERROR
dxffdkfs36op4ty09h65pid2f  keeper.1  sorintlab/stolon-keeper:0.5.0  swarm-worker-01  Running        Running 11 minutes ago
6tv7ohrcfvmb0k2d5li99nw9l  keeper.2  sorintlab/stolon-keeper:0.5.0  swarm-worker-02  Running        Running 11 minutes ago
0590gdetd56bx4prritou1tn2  keeper.3  sorintlab/stolon-keeper:0.5.0  swarm-worker-00  Running        Running 11 minutes ago
$ docker logs -f keeper.1.dxffdkfs36op4ty09h65pid2f
....
[I] 2016-12-26T04:20:55Z keeper.go:1124: our db requested role is master
[I] 2016-12-26T04:20:55Z postgresql.go:191: starting database
[I] 2016-12-26T04:20:56Z keeper.go:1144: already master

# stop the master Keeper container
$ docker stop keeper.1.dxffdkfs36op4ty09h65pid2f

# examine Sentinel's log
$ docker logs -f sentinel.1.95r09ncjyzgmdx501ven7dfnq
.....
[E] 2016-12-26T04:48:20Z sentinel.go:234: no keeper info available db=d106c3c6 keeper=76a713c4
[I] 2016-12-26T04:48:20Z sentinel.go:743: master db is failed db=d106c3c6 keeper=76a713c4
[I] 2016-12-26T04:48:20Z sentinel.go:754: trying to find a new master to replace failed master
[I] 2016-12-26T04:48:20Z sentinel.go:785: electing db as the new master db=3d665ac6 keeper=ef175c41
[E] 2016-12-26T04:48:25Z sentinel.go:234: no keeper info available db=5fe088f8 keeper=f02709c1
[E] 2016-12-26T04:48:25Z sentinel.go:234: no keeper info available db=d106c3c6 keeper=76a713c4
[E] 2016-12-26T04:48:30Z sentinel.go:234: no keeper info available db=5fe088f8 keeper=f02709c1
[E] 2016-12-26T04:48:30Z sentinel.go:234: no keeper info available db=d106c3c6 keeper=76a713c4
[I] 2016-12-26T04:48:30Z sentinel.go:844: removing old master db db=d106c3c6
.....
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

To remove all the services in the swarm, run `make swarm-clean`. This will stop and remove all etcd and stolon containers. By default, all the nodes aren't removed. To destroy all the nodes, use `make swarm-destroy`.

### Known Issues
At the time of this writing, there are no ways to view the services' logs directly in Docker 1.12. Refer this issue [here](https://github.com/portainer/portainer/issues/334). The workaround involves using `docker service ps` to determine which node the service task is scheduled to and run `docker logs` on that node.
