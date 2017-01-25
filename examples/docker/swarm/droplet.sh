#! /bin/bash

set -e

usage() {
  cat <<EOF
The following is the list of required variables:
  DO_ACCESS_TOKEN        DigitalOcean personal access token. Can be generated from the DigitalOcean web console.
  DO_REGION              Region of the droplets.
  DO_SIZE                Size of the droplets.
  DO_SSH_KEY_FINGERPRINT Fingerprint of the DigitalOcean SSH key used to access the droplets. The SSH key can be generated from the DigitalOcean web console.
EOF
}

if [ -z "$DO_ACCESS_TOKEN" ]
then
  echo "Can't create droplets. Missing required DO_ACCESS_TOKEN."
  usage
  exit 1
fi

if [ -z "$DO_REGION" ]
then
  echo "Can't create droplets. Missing required DO_REGION."
  usage
  exit 1
fi

if [ -z "$DO_SIZE" ]
then
  echo "Can't create droplets. Missing required DO_SIZE."
  usage
  exit 1
fi

if [ -z "$DO_SSH_KEY_FINGERPRINT" ]
then
  echo "Can't create droplets. Missing required DO_SSH_KEY_FINGERPRINT."
  usage
  exit 1
fi

declare -a droplets
droplets=(${SWARM_MANAGER} ${SWARM_WORKER_00} ${SWARM_WORKER_01} ${SWARM_WORKER_02})
for droplet in "${droplets[@]}"
do
  docker-machine create --driver digitalocean \
    --digitalocean-access-token ${DO_ACCESS_TOKEN} \
    --digitalocean-region ${DO_REGION} \
    --digitalocean-size ${DO_SIZE} \
    --digitalocean-private-networking \
    --digitalocean-ssh-key-fingerprint ${DO_SSH_KEY_FINGERPRINT} \
    $droplet
done
