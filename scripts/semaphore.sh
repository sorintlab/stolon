#!/bin/bash

set -e

# Setup for semaphore ci

if [ "${CI}" != "true" -o "${SEMAPHORE}" != "true" ]; then
	echo "not on semaphoreci"
	exit 1
fi

# Install and start etcd
mkdir etcd
pushd etcd
curl -L https://github.com/coreos/etcd/releases/download/v2.2.1/etcd-v2.2.1-linux-amd64.tar.gz -o etcd-v2.2.1-linux-amd64.tar.gz
tar xzvf etcd-v2.2.1-linux-amd64.tar.gz
popd

# Install and start etcd
mkdir consul
pushd consul
curl -L https://releases.hashicorp.com/consul/0.6.3/consul_0.6.3_linux_amd64.zip -o consul_0.6.3_linux_amd64.zip
unzip consul_0.6.3_linux_amd64.zip
popd


# Run tests
export ETCD_BIN="${PWD}/etcd/etcd-v2.2.1-linux-amd64/etcd"
export CONSUL_BIN="${PWD}/consul/consul"
export PATH=/usr/lib/postgresql/9.4/bin/:$PATH ; INTEGRATION=1 ./test
