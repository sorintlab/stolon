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

eval `docker-machine env swarm-manager`

network=stolon-network
docker network create -d overlay $network

declare -a etcd_services
etcd_services=(etcd-00 etcd-01 etcd-02)
for service in "${etcd_services[@]}"
do
  etcd_cluster=$etcd_cluster,$service=http://$service:2380
  etcd_endpoints=$etcd_endpoints,http://$service:2379
done

for service in "${etcd_services[@]}"
do
  docker service create --name $service --replicas 1 --network $network quay.io/coreos/etcd:$ETCD_VERSION \
    /usr/local/bin/etcd \
      --name=$service \
      --data-dir=data.etcd \
      --advertise-client-urls=http://$service:2379 \
      --listen-client-urls=http://0.0.0.0:2379 \
      --initial-advertise-peer-urls=http://$service:2380 \
      --listen-peer-urls=http://0.0.0.0:2380 \
      --initial-cluster=$etcd_cluster \
      --initial-cluster-state=new \
      --initial-cluster-token=$ETCD_TOKEN
done

docker service create --name sentinel --replicas 1 -e STSENTINEL_STORE_ENDPOINTS=$etcd_endpoints --network $network ${IMAGE_TAG_SENTINEL}
docker service create --name keeper --replicas 3 -e STKEEPER_STORE_ENDPOINTS=$etcd_endpoints --network $network ${IMAGE_TAG_KEEPER}
docker service create --name proxy --replicas 1 -e STPROXY_STORE_ENDPOINTS=$etcd_endpoints --network $network -p ${STOLON_PROXY_PORT} ${IMAGE_TAG_PROXY}
