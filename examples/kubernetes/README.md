# stolon inside kubernetes

In this example you'll see how stolon can provide an high available postgreSQL cluster inside kubernetes.


## Docker image
A prebuilt image is available on the dockerhub (sorintlab/stolon:0.1) and it's the one used by this example.

in the [image](examples/kubernetes/image/docker) directory you'll find the Dockerfile to build the image used in this example. Once the image is built you should push it to the docker registry used by your kubernetes infrastructure.


## cluster creation

These example points to a single node etcd cluster on `http://10.245.1.1:4001`. You can change the ST${COMPONENT}_ETCD_ENDPOINTS environment variables in the definitions to point to the right etcd cluster.

### Create the first stolon keeper
Note: In this example the stolon keeper is a replication controller that, for every pod replica, uses a volume for stolon and postgreSQL data of `emptyDir` type. So it'll go away when the related pod is destroyed. This is just for easy testing. In production you should use a persistent volume. Actually (kubernetes 1.0), for working with persistent volumes you should define a different replication controller with `replicas=1` for every keeper instance.


```
kubectl create -f stolon-keeper.yaml
```

This will create a replication controller that will create one pod executing the stolon keeper.


### Create the sentinel(s)

```
kubectl create -f stolon-sentinel.yaml
```

This will create a replication controller with one pod executing the stolon sentinel. You can also increase the number of replicas for stolon sentinel in the rc definition or do it later.

Once the leader sentinel has elected the first master and created the initial cluster view you can add additional stolon keepers


### Create the proxies

Also the proxies can have multiple replicas

```
kubectl create -f stolon-proxy.yaml
```

### Create the proxy service

The proxy service is used as entry point with a fixed ip and dns name for accessing the proxies.

```
kubectl create -f stolon-proxy-service.yaml
```

## Scale you cluster

From now on you can add additional stolon keepers and also increase/decrease the number of stolon sentinels and proxies.

```
kubectl scale --replicas=3 rc stolon-sentinel-rc
```

```
kubectl scale --replicas=3 rc stolon-proxy-rc
```
