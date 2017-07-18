#!/usr/bin/env bash

set -e

# Setup for semaphore ci

if [ "${CI}" != "true" -o "${SEMAPHORE}" != "true" ]; then
	echo "not on semaphoreci"
	exit 1
fi

# Install etcd
mkdir etcd
pushd etcd
curl -L https://github.com/coreos/etcd/releases/download/v3.1.8/etcd-v3.1.8-linux-amd64.tar.gz -o etcd-v3.1.8-linux-amd64.tar.gz
tar xzvf etcd-v3.1.8-linux-amd64.tar.gz
popd

# Install consul
mkdir consul
pushd consul
curl -L https://releases.hashicorp.com/consul/0.6.3/consul_0.6.3_linux_amd64.zip -o consul_0.6.3_linux_amd64.zip
unzip consul_0.6.3_linux_amd64.zip
popd

# Install postgreSQL 9.5 and 9.6
# TODO(sgotti) remove this when semaphoreci images will have this already installed
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main 10" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt-get update
sudo apt-get -y install postgresql-9.5 postgresql-9.6 postgresql-10

# Precompile stdlib with cgo disable to speedup builds
sudo -E CGO_ENABLED=0 go install -a -installsuffix cgo std

# Run tests
export ETCD_BIN="${PWD}/etcd/etcd-v3.1.8-linux-amd64/etcd"
export CONSUL_BIN="${PWD}/consul/consul"

OLDPATH=$PATH

# Increase sysv ipc semaphores to accomode a big number of parallel postgres instances
sudo /bin/sh -c 'echo "32000 1024000000 500 32000" > /proc/sys/kernel/sem'

# Free up some disk space
rm -rf ~/.rbenv

export INTEGRATION=1
export PARALLEL=20

# Test with postgresql 9.5
echo "===== Testing with postgreSQL 9.5 ====="
export PATH=/usr/lib/postgresql/9.5/bin/:$OLDPATH ; ./test

# Test with postgresql 9.6
echo "===== Testing with postgreSQL 9.6 ====="
export PATH=/usr/lib/postgresql/9.6/bin/:$OLDPATH ; ./test

# Test with postgresql 10
echo "===== Testing with postgreSQL 10 ====="
export PATH=/usr/lib/postgresql/10/bin/:$OLDPATH ; ./test
