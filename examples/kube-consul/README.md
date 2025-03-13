
helm template consul hashicorp/consul --set global.name=consul --create-namespace --namespace consul > consul.yaml
