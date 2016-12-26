#!/bin/bash

set -e

eval `docker-machine env ${SWARM_MANAGER}`
docker service rm sentinel keeper proxy etcd-00 etcd-01 etcd-02
docker network rm stolon-network

eval `docker-machine env ${SWARM_WORKER_00}`
docker rm -f `docker ps -a -q`

eval `docker-machine env ${SWARM_WORKER_01}`
docker rm -f `docker ps -a -q`

eval `docker-machine env ${SWARM_WORKER_02}`
docker rm -f `docker ps -a -q`

if [ "${DESTROY_MACHINES}" == true ]
then
  docker-machine rm ${SWARM_MANAGER} ${SWARM_WORKER_00} ${SWARM_WORKER_01} ${SWARM_WORKER_02}
fi
