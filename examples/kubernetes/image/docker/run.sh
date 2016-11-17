#!/bin/bash

function setup() {
  # use hostname command to get our pod's ip until downward api are less racy (sometimes the podIP from downward api is empty)
  export POD_IP=$(hostname -i)
}

function checkdata() {
  if [[ ! -e /stolon-data ]]; then
    echo "stolon data doesn't exist, data won't be persistent!"
    mkdir /stolon-data
  fi
  chown stolon:stolon /stolon-data
}

function launchkeeper() {
  checkdata
  export STKEEPER_PG_LISTEN_ADDRESS=$POD_IP
  su stolon -c "stolon-keeper --data-dir /stolon-data"
}

function launchsentinel() {
  stolon-sentinel
}

function launchproxy() {
  export STPROXY_LISTEN_ADDRESS=0.0.0.0
  stolon-proxy
}

echo "start"
setup
env

if [[ "${KEEPER}" == "true" ]]; then
  launchkeeper
  exit 0
fi

if [[ "${SENTINEL}" == "true" ]]; then
  launchsentinel
  exit 0
fi

if [[ "${PROXY}" == "true" ]]; then
  launchproxy
  exit 0
fi

exit 1
