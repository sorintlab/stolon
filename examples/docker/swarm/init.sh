#!/bin/bash

set -e

eval `docker-machine env ${SWARM_MANAGER}`
manager_ipv4=`docker-machine ip ${SWARM_MANAGER}`
docker swarm init --advertise-addr $manager_ipv4 --listen-addr $manager_ipv4
docker node update --availability drain $SWARM_MANAGER

join_token=`docker swarm join-token -q worker`
declare -a workers
workers=(${SWARM_WORKER_00} ${SWARM_WORKER_01} ${SWARM_WORKER_02})
for worker in "${workers[@]}"
do
  eval `docker-machine env $worker`
  ipv4_address=`docker-machine ip $worker`
  docker swarm join --token $join_token --advertise-addr $ipv4_address --listen-addr $ipv4_address $manager_ipv4:2377
done
