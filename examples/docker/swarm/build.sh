#!/bin/bash

set -e

declare -a machines
machines=(${SWARM_MANAGER} ${SWARM_WORKER_00} ${SWARM_WORKER_01} ${SWARM_WORKER_02})
for machine in "${machines[@]}"
do
  eval `docker-machine env $machine`
  make sentinel keeper proxy
done
