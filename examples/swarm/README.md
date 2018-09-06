# Stolon inside docker swarm

In this example you'll see how stolon can provide an high available postgreSQL cluster inside Docker swarm.

We will use etcdv3 deployed in its own stack. 

## Docker image

Prebuilt images are available on the dockerhub, the images' tags are the stolon release version plus the postgresql version (for example v0.10.0-pg10).

**NOTE**: These images are **example** images provided for quickly testing stolon. In production you should build your own image customized to fit your needs (adding postgres extensions, backup tools/scripts etc...).

Additional images are available:

* `master-pg9.6`: automatically built after every commit to the master branch.

In the [image](../kubernetes/image/docker) directory you'll find a Makefile to build the image used in this example (starting from the official postgreSQL images). The Makefile generates the Dockefile from a template Dockerfile where you have to define the wanted postgres version and image tag (`PGVERSION` and `TAG` mandatory variables).
For example, if you want to build an image named `stolon:master-pg9.6` that uses postgresql 9.6 you should execute:

```
make PGVERSION=10.3 TAG=stolon:master-pg10
```

Once the image is built you should push it to the docker registry used by your swarm infrastructure.

The provided example uses `sorintlab/stolon:master-pg10`


## Cluster setup and tests

This example has some predefined values that you'd like to change:

* The cluster name is `stolon-cluster`. It's set in the component `--cluster-name` option. The labels and the `--cluster-name` option must be in sync.
* It uses the etcdv3 backend. You can also choose other backends (like zookeeper) setting the `ST${COMPONENT}_STORE_*` environment variables (see the [commands invocation documentation](/doc/commands_invocation.md)).

### Initialize the swarm

If you don't already have one, you can create a new swarm using the following command:

```
docker swarm init 
```
The output of the command above should provide you the command to use to add worker nodes.
Something like:

```
docker swarm join --token <TOKEN> <IP>:<PORT>  
```

It's highly recommended to set labels on each node so that we can set deployment constraints.
You can use to following command to set a `nodename` label equal to `node1` on the first node:

```
docker node update --label-add nodename=node1 <NODE> 
```

Repeat this command to set a different label for each node. Allowed values for `<NODE>` can be found using this command:
```
docker node ls 
```

 
### Initialize the etcdv3 backend

Now, before starting any stolon component, we need to start the etcdv3 backend. This can easily be done using the provided `docker-compose-etcd.yml` file:

```
docker stack deploy --compose-file docker-compose-etcd.yml etcd
```

You can check everything is fine using this command:

```
docker service ls
ID                  NAME                MODE                REPLICAS            IMAGE                         PORTS
pbjr4k9285iy        etcd_etcd-00        replicated          1/1                 quay.io/coreos/etcd:v3.2.10   *:2379->2379/tcp
7bc4d0e0qf2y        etcd_etcd-01        replicated          1/1                 quay.io/coreos/etcd:v3.2.10   
2dcqqmi5li0v        etcd_etcd-02        replicated          1/1                 quay.io/coreos/etcd:v3.2.10 
```

### Initialize the cluster

All the stolon components wait for an existing clusterdata entry in the store. So the first time you have to initialize a new cluster. For more details see the [cluster initialization doc](/doc/initialization.md). You can do this step at every moment, now or after having started the stolon components.

You can execute stolonctl from a machine that can access the store backend:

```
stolonctl --cluster-name=stolon-cluster --store-backend=etcdv3 --store-endpoints http://localhost:2379 init
```

### Create sentinel(s), keepers and proxy(ies)

To create all the stolon components, we just have to create a new stack based on the file `docker-compose-pg.yml`.
Before creating the stack, in case you have several nodes, it's highly recommended to set constraints so that each keeper
will start on its own node.
Assuming you have 2 nodes with labels `nodename=node1` and `nodename=node2`, just edit the file `docker-compose-pg.yml` 
and uncomment the lines:
```
      placement:
        constraints: [node.labels.nodename == node1]
```
and
```
      placement:
        constraints: [node.labels.nodename == node2]
```

If you have only one node, just leave the file unchanged.
Now, enter the following command to create the new stack with all stolon components:

```
docker stack deploy --compose-file docker-compose-pg.yml pg
```

This will create 2 sentinels, 2 proxies and 2 keepers. 
You can check everything is fine using this command:

```
docker service ls
ID                  NAME                MODE                REPLICAS            IMAGE                          PORTS
pbjr4k9285iy        etcd_etcd-00        replicated          1/1                 quay.io/coreos/etcd:v3.2.17    *:2379->2379/tcp
7bc4d0e0qf2y        etcd_etcd-01        replicated          1/1                 quay.io/coreos/etcd:v3.2.17    
2dcqqmi5li0v        etcd_etcd-02        replicated          1/1                 quay.io/coreos/etcd:v3.2.17    
k2vh3ff0acpg        pg_keeper1          replicated          1/1                 sorintlab/master-pg10:latest   
jbm195xsalwu        pg_keeper2          replicated          1/1                 sorintlab/master-pg10:latest   
sf79wnygtcmu        pg_proxy            replicated          2/2                 sorintlab/master-pg10:latest   *:5432->5432/tcp
tkbnrfdj4axa        pg_sentinel         replicated          2/2                 sorintlab/master-pg10:latest
```

### Connect to the db

#### Connect to the proxy service

The password for the stolon user will be the value specified in your `./etc/secrets/pgsql` file (or `password1` if you did not change it). 

```
psql --host localhost --port 5432 postgres -U postgres -W
Password for user postgres:
psql (10.3, server 10.3)
Type "help" for help.

postgres=#
```

### Create a test table and insert a row

```
postgres=# create table test (id int primary key not null, value text not null);
CREATE TABLE
postgres=# insert into test values (1, 'value1');
INSERT 0 1
postgres=# select * from test;
 id | value
----+--------
  1 | value1
(1 row)
```

### Simulate master death

There are different ways to tests this. In a multi node setup you can just shutdown the host executing the master keeper service.

In a single node setup we can kill the current master keeper service but the swarm will restart it before the sentinel declares it as failed.
To avoid the restart, we'll scale to 0 replica the master keeper. Assuming it is keeper1, use the following command:

```
docker service scale pg_keeper1=0
```

You can take a look at the leader sentinel log and will see that after some seconds it'll declare the master keeper as not healthy and elect the other one as the new master:
```
no keeper info available db=cb96f42d keeper=keeper0
no keeper info available db=cb96f42d keeper=keeper0
master db is failed db=cb96f42d keeper=keeper0
trying to find a standby to replace failed master
electing db as the new master db=087ce88a keeper=keeper1
```

Now, inside the previous `psql` session you can redo the last select. The first time `psql` will report that the connection was closed and then it successfully reconnected:

```
postgres=# select * from test;
server closed the connection unexpectedly
        This probably means the server terminated abnormally
        before or while processing the request.
The connection to the server was lost. Attempting reset: Succeeded.
postgres=# select * from test;
 id | value
----+--------
  1 | value1
(1 row)
```

### Scale your cluster keepers

You can add additional stolon keepers by duplicating an existing keeper definition. You need to specify a dedicated volume for each keeper.

### Scale your cluster sentinels and proxies

You can increase/decrease the number of stolon sentinels and proxies:

```
docker service scale pg_sentinel=3
```

```
docker service scale pg_proxy=3
```

### Update image

For PostgreSQL major version upgrade, see [PostgreSQL upgrade](postgresql_upgrade.md)

For any PostgreSQL upgrade, check PostgreSQL release note for any additional upgrade note.

For stolon upgrade: TODO
