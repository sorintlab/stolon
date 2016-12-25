.PHONY: build secrets local-up local-clean

include .env

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

compose-up:
	@test -n "${ETCD_TOKEN}" # Please provide a value for $${ETCD_TOKEN}
	docker-compose up -d --remove-orphans

compose-down:
	docker-compose down --remove-orphans

compose-purge:
	docker-compose down -v --remove-orphans
