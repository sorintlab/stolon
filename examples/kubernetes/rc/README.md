# Stolon inside kubernetes

**DEPRECATED EXAMPLE**: Please use the [statefulset](../statefulset/README.md) example. Using persistent volumes with replication controller/replica sets won't guarantee at most once pod existence and can lead to data corruption with some persistent volume kind that can end attached in rw mode to more than one node at a time.

This is a simple example that uses replication controller for all stolon components.

Since the keeper requires a persistent data directory we define a replication controller with replicas=1 for each keeper. The keeper id is fixed inside the pod template definition so it won't be generated as a unique id but will have a more meaningful name (`keeper0`, `keeper1` etc...).
The replication controller is used, instead of a bare pod definition, to handle automatic pod restarts when the pod or the executing node dies.

## Cluster setup and tests

This example has some predefined values that you'd like to change:
* The cluster name is `kube-stolon`
* It points to a single node etcd cluster on `192.168.122.1:2379` without tls. You can change the `ST${COMPONENT}_STORE_ENDPOINTS` environment variables in the definitions to point to the right etcd cluster.

### Initialize the cluster

The first step is to initialize a cluster with a cluster specification. For now we'll just initialize a cluster without providing a cluster specification but using a default one that will just start with an empty database cluster.

```
./bin/stolonctl --cluster-name=kube-stolon --store-backend=etcd init
```

If you want to automate this step you can just pass an initial cluster specification to the sentinels with the `--initial-cluster-spec` option.

### Create the sentinel(s)

```
kubectl create -f stolon-sentinel.yaml
```

This will create a replication controller with two pod replicas executing the stolon sentinel. You can also increase the number of replicas for stolon sentinels in the rc definition or do it later.

### Create the keeper's password secret

This creates a password secret that can be used by the keeper to set up the initial database superuser. This example uses the value 'password1' but you will want to replace the value with a Base64-encoded password of your choice.

```
kubectl create -f secret.yaml
```

### Create the stolon keepers

The example definition uses a `hostPath` persistent volume but this shouldn't be used in production and won't work in multi-node cluster. In production you should use a persistentVolumeClaim and configure your persistent volumes (statically or dynamic using a provisioner, see k8s doc).

```
kubectl create -f stolon-keeper0.yaml
kubectl create -f stolon-keeper1.yaml
```

The sentinel will choose a random keeper as the initial master, this keeper will initialize a new db cluster and the other keeper will become a standby.

### Create the proxies

```
kubectl create -f stolon-proxy.yaml
```

This will create a replication controller that defines 2 replicas for the stolon proxy. You can change the number of replicas in the rc definit (or scale it with `kubectl scale`).

### Create the proxy service

The proxy service is used as an entry point with a fixed ip and dns name for accessing the proxies.

```
kubectl create -f stolon-proxy-service.yaml
```

### Connect to the db

#### Get the proxy service ip

```
kubectl get svc
NAME                   LABELS                                    SELECTOR                                       IP(S)           PORT(S)
stolon-proxy-service   <none>                                    stolon-cluster=kube-stolon,stolon-proxy=true   10.247.50.217   5432/TCP
```

#### Connect to the proxy service

The password for the stolon user will be the value specified in your `secret.yaml` above (or `password1` if you did not change it). 

```
psql --host 10.247.50.217 --port 5432 postgres -U stolon -W
Password for user stolon:
psql (9.4.5, server 9.4.4)
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

you'll have a state like this:

```
kubectl get pods
NAME                       READY     STATUS    RESTARTS   AGE
stolon-keeper0-qpqp9       1/1       Running   0          5m
stolon-keeper1-2fvw1       1/1       Running   0          5m
stolon-proxy-up3x0         1/1       Running   0          5m
stolon-sentinel-9cvxm      1/1       Running   0          5m
```

### Simulate master death
There are different ways to tests this. In a multi node setup you can just shutdown the host executing the master keeper pod.

In a single node setup we can kill the current master keeper pod but usually the replication controller will recreate a new pod before the sentinel declares it as failed.

To avoid the restart we'll delete the master keeper rc (and so its pod will be deleted). The data will be kept so we'll be able to recreate the replication controller and the related pod will be recreated with the previous data.

```
kubectl delete rc stolon-keeper0
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

### Scale your cluster

You can add additional stolon keepers defining a new replication controller and chaning the parameters that must be unique for that keeper (keeper id and data volume).

You can increase/decrease the number of stolon sentinels and proxies:

```
kubectl scale --replicas=3 rc stolon-sentinel
```

```
kubectl scale --replicas=3 rc stolon-proxy
```

### Update image

TODO

