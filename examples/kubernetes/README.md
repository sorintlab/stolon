# Stolon inside kubernetes

In this example you'll see how stolon can provide an high available postgreSQL cluster inside kubernetes.

The sentinels and proxies will be deployed as [kubernetes deployments](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/) while the keepers as a [kubernetes statefulset](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/).

## Docker image

Prebuilt images are available on the dockerhub, the images' tags are the stolon release version plus the postgresql version (for example v0.6.0-pg9.6). Additional images are available:

* `master-pg9.6`: automatically built after every commit to the master branch.

In the [image](image/docker) directory you'll find a Makefile to build the image used in this example (starting from the official postgreSQL images). The Makefile generates the Dockefile from a template Dockerfile where you have to define the wanted postgres version and image tag (`PGVERSION` and `TAG` mandatory variables).
For example, if you want to build an image named `stolon:master-pg9.6` that uses postgresql 9.6 you should execute:

```
make PGVERSION=9.6 TAG=stolon:master-pg9.6
```

Once the image is built you should push it to the docker registry used by your kubernetes infrastructure.

The provided example uses `sorintlab/stolon:master-pg9.6`


## Cluster setup and tests

This example has some predefined values that you'd like to change:

* The cluster name is `kube-stolon`. It's set in the various `stolon-cluster` labels and in the component `--cluster-name` option. The labels and the `--cluster-name` option must be in sync.
* It uses the kubernetes backend. You can also choose other backends (like etcdv3) setting the `ST${COMPONENT}_STORE_*` environment variables (see the [commands invocation documentation](/doc/commands_invocation.md)).

If your k8s cluster has RBAC enabled you should create a role and a rolebinding to a service account. As an example take a look at the provided [role](role.yaml) and [role-binding](role-binding.yaml) example definitions that define a `stolon` role bound to the `default` service account in the `default` namespace.

### Initialize the cluster

All the stolon components wait for an existing clusterdata entry in the store. So the first time you have to initialize a new cluster. For more details see the [cluster initialization doc](/doc/initialization.md). You can do this step at every moment, now or after having started the stolon components.

You can execute stolonctl in different ways:

* as a one shot command executed inside a temporary pod:

```
kubectl run -i -t stolonctl --image=sorintlab/stolon:master-pg9.6 --restart=Never --rm -- /usr/local/bin/stolonctl --cluster-name=kube-stolon --store-backend=kubernetes --kube-resource-kind=configmap init
```

* from a machine that can access the store backend:

```
stolonctl --cluster-name=kube-stolon --store-backend=kubernetes --kube-resource-kind=configmap init
```

* later from one of the pods running the stolon components.


### Create the sentinel(s)

```
kubectl create -f stolon-sentinel.yaml
```

This will create a deployment that defines 2 replicas for the stolon sentinel. You can change the number of replicas in the deployment definition (or scale it with `kubectl scale`).

### Create the keeper's password secret

This creates a password secret that can be used by the keeper to set up the initial database superuser. This example uses the value 'password1' but you will want to replace the value with a Base64-encoded password of your choice.

```
kubectl create -f secret.yaml
```

### Create the stolon keepers statefulset

The example definition uses a dynamic provisioning with a storage class of type "anything" that works also with minikube and will provision volume using the hostPath provider, but this shouldn't be used in production and won't work in multi-node cluster.
In production you should use your own defined storage-class and configure your persistent volumes (statically or dynamic using a provisioner, see the related k8s documentation).

```
kubectl create -f stolon-keeper.yaml
```

This will define a statefulset that will create 2 stolon-keepers.
The sentinel will choose a random keeper as the initial master, this keeper will initialize a new db cluster and the other keeper will become a standby.

### Create the proxies

```
kubectl create -f stolon-proxy.yaml
```

This will create a deployment that defines 2 replicas for the stolon proxy. You can change the number of replicas in the deployment definition (or scale it with `kubectl scale`).

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
NAME                               READY     STATUS    RESTARTS   AGE
stolon-keeper-0                    1/1       Running   0          5m
stolon-keeper-1                    1/1       Running   0          5m
stolon-proxy-fd7c9b4bd-89c9z       1/1       Running   0          5m
stolon-proxy-fd7c9b4bd-pmj86       1/1       Running   0          5m
stolon-sentinel-5c76865bd5-bc9n2   1/1       Running   0          5m
stolon-sentinel-5c76865bd5-fmqts   1/1       Running   0          5m
```

### Simulate master death
There are different ways to tests this. In a multi node setup you can just shutdown the host executing the master keeper pod.

In a single node setup we can kill the current master keeper pod but usually the statefulset controller will recreate a new pod before the sentinel declares it as failed.
To avoid the restart we'll first remove the statefulset without removing the pod and then kill the master keeper pod. The persistent volume will be kept so we'll be able to recreate the statefulset and the missing pods will be recreated with the previous data.


```
kubectl delete statefulset stolon-keeper --cascade=false
kubectl delete pod stolon-keeper-0
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

You can add additional stolon keepers increasing the replica count in the statefulset. Shrinking the statefulset should be done very carefully or you can end in a situation where the current master pod will be removed and the remaining keepers cannot be elected as master because not in sync.

### Scale your cluster sentinels and proxies

You can increase/decrease the number of stolon sentinels and proxies:

```
kubectl scale --replicas=3 deployment stolon-sentinel
```

```
kubectl scale --replicas=3 deployment stolon-proxy
```

### Update image

For PostgreSQL major version upgrade, see [PostgreSQL upgrade](postgresql_upgrade.md)

For any PostgreSQL upgrade, check PostgreSQL release note for any additional upgrade note.

For stolon upgrade: TODO

