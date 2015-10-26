#!/bin/bash

set -e

# Setup for semaphore ci

if [ "${CI}" != "true" -o "${SEMAPHORE}" != "true" ]; then
	echo "not on semaphoreci"
	exit 1
fi

# Install and start etcd
mkdir etcd
cd etcd
curl -L  https://github.com/coreos/etcd/releases/download/v2.2.1/etcd-v2.2.1-linux-amd64.tar.gz -o etcd-v2.2.1-linux-amd64.tar.gz
tar xzvf etcd-v2.2.1-linux-amd64.tar.gz
cd etcd-v2.2.1-linux-amd64
start-stop-daemon -b --start --exec $PWD/etcd -- --data-dir=$PWD/
cd ../../

# Run tests
export PATH=/usr/lib/postgresql/9.4/bin/:$PATH ; INTEGRATION=1 ./test
