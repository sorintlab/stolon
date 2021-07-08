#!/usr/bin/env bash

set -e

apk add make jq

echo -n "Waiting for docker to be ready"
until curl -s --fail http://127.0.0.1:10080/docker-ready; do
    sleep 1;
    echo -n "."
done
echo " Ready"

cd /stolon

make PGVERSION=11 TAG=stolon:master-pg11 docker

pushd examples/kubernetes

# TODO(sgotti) bsycorp kind:v1.19.4 doesn't correctly parse kubectl output and will never report kubernetes in ready state
#echo -n "Waiting for kubernetes to be ready"
#until curl -s --fail http://127.0.0.1:10080/kubernetes-ready; do
#    sleep 1;
#    echo -n "."
#done
#echo " Ready"

sed -i 's#sorintlab/stolon:master-pg10#stolon:master-pg11#' *.yaml

for i in role.yaml role-binding.yaml secret.yaml stolon-sentinel.yaml stolon-keeper.yaml stolon-proxy.yaml stolon-proxy-service.yaml ; do
	kubectl apply -f $i
done

popd

KUBERUN="kubectl run --quiet -i -t stolonctl --image=stolon:master-pg11 --restart=Never --rm --"

$KUBERUN /usr/local/bin/stolonctl --cluster-name=kube-stolon --store-backend=kubernetes --kube-resource-kind=configmap init -y

OK=false
COUNT=0
while [ $COUNT -lt 120 ]; do
	OUT=$($KUBERUN /usr/local/bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap clusterdata read | jq .cluster.status.phase)
	if [ "$OUT" == '"normal"' ]; then
		OK=true
		break
	fi

	COUNT=$((COUNT + 1))
	sleep 1
done

# report some debug output
kubectl get all
$KUBERUN /usr/local/bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap status
$KUBERUN /usr/local/bin/stolonctl --cluster-name kube-stolon --store-backend kubernetes --kube-resource-kind configmap clusterdata read | jq .

if [ "$OK" != "true" ]; then
	echo "stolon cluster not correctly setup"
	exit 1
fi

echo "stolon cluster successfully setup"
