.PHONY: build secrets local-up local-clean

ETCD_VERSION ?= v3.0.15

STOLON_CLUSTER_NAME ?= stolon-cluster
STOLON_STORE_BACKEND ?= etcd
STOLON_PROXY_PORT ?= 25432

IMAGE_TAG_PREFIX ?= sorintlab
IMAGE_TAG_SUFFIX ?= 0.5.0
IMAGE_TAG_SENTINEL = ${IMAGE_TAG_PREFIX}/stolon-sentinel:${IMAGE_TAG_SUFFIX}
IMAGE_TAG_KEEPER = ${IMAGE_TAG_PREFIX}/stolon-keeper:${IMAGE_TAG_SUFFIX}
IMAGE_TAG_PROXY = ${IMAGE_TAG_PREFIX}/stolon-proxy:${IMAGE_TAG_SUFFIX}

export ETCD_VERSION ETCD_TOKEN IMAGE_TAG_SENTINEL IMAGE_TAG_KEEPER IMAGE_TAG_PROXY STOLON_PROXY_PORT

build: secrets sentinel keeper proxy

sentinel:
	docker build --rm -t ${IMAGE_TAG_SENTINEL} -f Dockerfile-Sentinel .

keeper:
	docker build --rm -t ${IMAGE_TAG_KEEPER} -f Dockerfile-Keeper .

proxy:
	docker build --rm -t ${IMAGE_TAG_PROXY} -f Dockerfile-Proxy .

secrets:
	rm -rf etc/secrets
	mkdir -p etc/secrets
	cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1 >> etc/secrets/pgsql
	cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1 >> etc/secrets/pgsql-repl

local-up:
	@test -n "${ETCD_TOKEN}" # Please provide a value for $${ETCD_TOKEN}
	./local-up.sh

local-clean:
	./local-clean.sh

local-purge:
	PURGE_VOLUMES=true ./local-clean.sh
