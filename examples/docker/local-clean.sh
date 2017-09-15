#!/bin/bash

if [ "$PURGE_VOLUMES" ]
then
  purge_volumes="-v"
fi

docker rm -f $purge_volumes stolon-sentinel stolon-proxy

declare -a etcd_nodes
etcd_nodes=(etcd-00 etcd-01 etcd-02)
for node in "${etcd_nodes[@]}"
do
  docker rm -f $purge_volumes $node
done

declare -a keepers
keepers=(stolon-keeper-00 stolon-keeper-01 stolon-keeper-02)
for keeper in "${keepers[@]}"
do
  docker rm -f $purge_volumes $keeper
done

docker network rm stolon-network
