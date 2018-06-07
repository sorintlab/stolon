#!/usr/bin/env bash

set -e

# Setup for semaphore ci

if [ "${CI}" != "true" -o "${SEMAPHORE}" != "true" ]; then
	echo "not on semaphoreci"
	exit 1
fi

# Free up some disk space
rm -rf ~/.rbenv ~/.php*

# Install kubectl and minikube
curl -Lo minikube https://github.com/kubernetes/minikube/releases/download/v0.25.2/minikube-linux-amd64 && chmod +x minikube && sudo cp minikube /usr/local/bin/
curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl && sudo cp kubectl /usr/local/bin/

# Start local minikube
export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export MINIKUBE_HOME=$HOME
export CHANGE_MINIKUBE_NONE_USER=true
mkdir $HOME/.kube || true
touch $HOME/.kube/config

export KUBECONFIG=$HOME/.kube/config
sudo -E minikube start --vm-driver=none

# Precompile stdlib with cgo disable to speedup builds
sudo -E CGO_ENABLED=0 go install -a -installsuffix cgo std

./build


pushd examples/kubernetes/image/docker

make PGVERSION=10 TAG=stolon:master-pg10

popd

./bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap init -y

pushd examples/kubernetes

sed -i 's#sorintlab/stolon:master-pg10#stolon:master-pg10#' *.yaml

for i in secret.yaml stolon-sentinel.yaml stolon-keeper.yaml stolon-proxy.yaml stolon-proxy-service.yaml ; do
	kubectl apply -f $i
done

popd


OK=false
COUNT=0
while [ $COUNT -lt 120 ]; do
	OUT=$(./bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap clusterdata | jq .cluster.status.phase)
	if [ "$OUT" == '"normal"' ]; then
		OK=true	
		break
	fi

	COUNT=$((COUNT + 1))
	sleep 1
done

# report some debug output
kubectl get all
./bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap status
./bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap clusterdata | jq .

if [ "$OK" != "true" ]; then
	echo "stolon cluster not correctly setup"
	exit 1
fi

echo "stolon cluster successfully setup"

