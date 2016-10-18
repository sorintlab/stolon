#!/bin/bash

set -e

# Setup for semaphore ci

if [ "${CI}" != "true" -o "${SEMAPHORE}" != "true" ]; then
	echo "not on semaphoreci"
	exit 1
fi

# Install etcd
mkdir etcd
pushd etcd
curl -L https://github.com/coreos/etcd/releases/download/v2.2.1/etcd-v2.2.1-linux-amd64.tar.gz -o etcd-v2.2.1-linux-amd64.tar.gz
tar xzvf etcd-v2.2.1-linux-amd64.tar.gz
popd

# Install consul
mkdir consul
pushd consul
curl -L https://releases.hashicorp.com/consul/0.6.3/consul_0.6.3_linux_amd64.zip -o consul_0.6.3_linux_amd64.zip
unzip consul_0.6.3_linux_amd64.zip
popd

# Install postgreSQL 9.5
# TODO(sgotti) remove this when semaphoreci images will have this already installed
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt-get update
sudo apt-get -y install postgresql-9.5

# Precompile stdlib with cgo disable to speedup builds
sudo -E CGO_ENABLED=0 go install -a -installsuffix cgo std

# Run tests
export ETCD_BIN="${PWD}/etcd/etcd-v2.2.1-linux-amd64/etcd"
export CONSUL_BIN="${PWD}/consul/consul"

OLDPATH=$PATH

# Test with postgresql 9.4
echo "===== Testing with postgreSQL 9.4 ====="
export PATH=/usr/lib/postgresql/9.4/bin/:$OLDPATH ; INTEGRATION=1 ./test

# Test with postgresql 9.5
echo "===== Testing with postgreSQL 9.5 ====="
export PATH=/usr/lib/postgresql/9.5/bin/:$OLDPATH ; INTEGRATION=1 ./test
