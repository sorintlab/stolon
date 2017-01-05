#!/bin/bash

set -e

declare -a vms
vms=(${SWARM_MANAGER} ${SWARM_WORKER_00} ${SWARM_WORKER_01} ${SWARM_WORKER_02})
for vm in "${vms[@]}"
do
  docker-machine create --driver virtualbox $vm
done
