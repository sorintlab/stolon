#!/bin/bash

set -e

usage() {
  cat <<EOF
The following is the list of required variables:
  ETCD_VERSION         Etcd Docker image version.
  ETCD_TOKEN           Initial cluster token for the etcd cluster during bootstrap.
  IMAGE_TAG_SENTINEL   Stolon Sentinel Docker image tag.
  IMAGE_TAG_KEEPER     Stolon Keeper Docker image tag.
  IMAGE_TAG_PROXY      Stolon Proxy Docker image tag.
  STOLON_PROXY_PORT    Port that Stolon Proxy listens on.
EOF
}

if [ -z "$ETCD_VERSION" ]
then
  echo "Can't create etcd cluster. Missing required ETCD_VERSION."
  usage
  exit 1
fi

if [ -z "$ETCD_TOKEN" ]
then
  echo "Can't create etcd cluster. Missing required ETCD_TOKEN."
  usage
  exit 1
fi

if [ -z "$IMAGE_TAG_SENTINEL" ]
then
  echo "Can't create stolon cluster. Missing required IMAGE_TAG_SENTINEL."
  usage
  exit 1
fi

if [ -z "$IMAGE_TAG_KEEPER" ]
then
  echo "Can't create stolon cluster. Missing required IMAGE_TAG_KEEPER."
  usage
  exit 1
fi

if [ -z "$IMAGE_TAG_PROXY" ]
then
  echo "Can't create stolon cluster. Missing required IMAGE_TAG_PROXY."
  usage
  exit 1
fi

if [ -z "$STOLON_PROXY_PORT" ]
then
  echo "Can't create stolon cluster. Missing required STOLON_PROXY_PORT."
  usage
  exit 1
fi

network=stolon-network
docker network create -d bridge $network

declare -a etcd_nodes
etcd_nodes=(etcd-00 etcd-01 etcd-02)
for node in "${etcd_nodes[@]}"
do
  etcd_cluster=$etcd_cluster,$node=http://$node:2380
  etcd_endpoints=$etcd_endpoints,http://$node:2379
done

for node in "${etcd_nodes[@]}"
do
  docker run --net=$network --name $node -d quay.io/coreos/etcd:$ETCD_VERSION \
    /usr/local/bin/etcd \
    --name $node \
    --data-dir=data.etcd \
    --advertise-client-urls http://$node:2379 \
    --listen-client-urls http://0.0.0.0:2379 \
    --initial-advertise-peer-urls http://$node:2380 \
    --listen-peer-urls http://0.0.0.0:2380 \
    --initial-cluster $etcd_cluster \
    --initial-cluster-state new \
    --initial-cluster-token $ETCD_TOKEN
done

docker run --name stolon-sentinel -d --net=$network -e STSENTINEL_STORE_ENDPOINTS=$etcd_endpoints $IMAGE_TAG_SENTINEL

declare -a keepers
keepers=(stolon-keeper-00 stolon-keeper-01 stolon-keeper-02)
for keeper in "${keepers[@]}"
do
  docker run --name $keeper -d --net=$network \
    -v $PWD/etc/secrets/pgsql:$STOLON_KEEPER_PG_SU_PASSWORDFILE \
    -v $PWD/etc/secrets/pgsql:$STOLON_KEEPER_PG_REPL_PASSWORDFILE \
    -e STKEEPER_STORE_ENDPOINTS=$etcd_endpoints \
    -e STKEEPER_PG_SU_PASSWORDFILE=$STOLON_KEEPER_PG_SU_PASSWORDFILE \
    -e STKEEPER_PG_REPL_PASSWORDFILE=$STOLON_KEEPER_PG_REPL_PASSWORDFILE \
    $IMAGE_TAG_KEEPER
done

docker run --name stolon-proxy -d --net=$network -p $STOLON_PROXY_PORT -e STPROXY_STORE_ENDPOINTS=$etcd_endpoints $IMAGE_TAG_PROXY
