## Stolon Client (stolonctl)

`stolonctl` is the stolon client which controls the stolon cluster(s). 

Since `stolonctl`needs to communicate with the cluster backend store, it requires providing the requested cluster name (`--cluster-name`), its store backend type (`--store-backend`), and how to reach the store, such as:
* For etcdv2, etcdv3 or consul as store, a comma separated list of endpoints (`--store-endpoints`).
* For kubernetes as store, the kind of kubernetes resources (`--kube-resource-kind`). See below.

`stolonctl` example for checking the status of a cluster named "stolon-cluster" using "etcdv3" as a store backend:
```
$ stolonctl --cluster-name=stolon-cluster --store-backend=etcdv3 --store-endpoints=http://etcd-0:2379,http://etcd-1:2379,http://etcd-2:2379 status
```

Note: To avoid repeating the arguments on every command (or inside scripts), all the options can be exported as environment variables. Their name will be the same as the option name converted in uppercase, with `_` replacing `-` and prefixed with `STOLONCTL_`.

For example:
```
STOLONCTL_STORE_BACKEND
STOLONCTL_STORE_ENDPOINTS
STOLONCTL_CLUSTER_NAME
```

### Running in Kubernetes

`stolonctl` behaves like [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) when choosing how to access the kubernetes API server(s): 
* When run inside a pod it uses the pod service account to connect to the k8s API servers.
* When run externally it honors the $KUBECONFIG environment variable to connect. It is thus possible to use the default `~/.kube/config` file or an overriden kube-config file path, context and namespace to set the `stolonctl` options `--kubeconfig`, `--kube-context` and `--kube-namespace`.

`stolonctl` example for checking the status of a cluster named "kube-stolon" using "kubernetes" as a store backend and "configmap" as the resource kind where the `stolonctl` command is invoked via one of the stolon proxy pods: 
```
$kubectl exec -i -t stolon-proxy-669f7b54fd-9psm2 -- stolonctl --cluster-name=kube-stolon --store-backend=kubernetes --kube-resource-kind=configmap status
```

Same `stolonctl` command as a one shot:
```
kubectl run -i -t stolonctl --image=sorintlab/stolon:master-pg9.6 --restart=Never --rm -- /usr/local/bin/stolonctl --cluster-name=kube-stolon --store-backend=kubernetes --kube-resource-kind=configmap status
```

### See also

[stolonctl command invocation](commands/stolonctl.md)
