# Stolon inside kubernetes

Here you can find some examples on running stolon inside kubernetes

There're two examples. The difference between them is how the keepers pods are deployed (the definitions of the other components is identical):

* Using a [statefulset](statefulset) (called `petset` in k8s 1.4)
* Using [replication controllers](rc) (one per keeper).

## Docker image

Prebuilt images are available on the dockerhub, the images' tags are the stolon release version plus the postgresql version (for example v0.6.0-pg9.6). Additional images are available:

* `latest-pg9.6`: latest released image (for stolon versions >= v0.5.0).
* `master-pg9.6`: automatically built after every commit to the master branch.


In the [image](examples/kubernetes/image/docker) directory you'll find a Makefile to build the image used in this example (starting from the official postgreSQL images). The Makefile generates the Dockefile from a template Dockerfile where you have to define the wanted postgres version and image tag (`PGVERSION` adn `TAG` mandatory variables).
For example, if you want to build an image named `stolon:master-pg9.6` that uses postgresql 9.6 you should execute:

```
make PGVERSION=9.6 TAG=stolon:master-pg9.6
```

Once the image is built you should push it to the docker registry used by your kubernetes infrastructure.

The provided example uses `sorintlab/stolon:master-pg9.6`
