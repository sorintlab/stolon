PROJDIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))

# change to project dir so we can express all as relative paths
$(shell cd $(PROJDIR))

REPO_PATH=github.com/pgvillage-tools/stolon

PGVERSION ?= 18

VERSION ?= $(shell scripts/git-version.sh)

LD_FLAGS="-w -X $(REPO_PATH)/cmd.Version=$(VERSION)"

$(shell mkdir -p bin )

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

.PHONY: all
all: build

.PHONY: build
build: sentinel keeper proxy stolonctl

.PHONY: sentinel keeper proxy stolonctl docker

keeper:
	GO111MODULE=on go build -ldflags $(LD_FLAGS) -o $(PROJDIR)/bin/stolon-keeper $(REPO_PATH)/cmd/keeper

sentinel:
	CGO_ENABLED=0 GO111MODULE=on go build -ldflags $(LD_FLAGS) -o $(PROJDIR)/bin/stolon-sentinel $(REPO_PATH)/cmd/sentinel

proxy:
	CGO_ENABLED=0 GO111MODULE=on go build -ldflags $(LD_FLAGS) -o $(PROJDIR)/bin/stolon-proxy $(REPO_PATH)/cmd/proxy

stolonctl:
	CGO_ENABLED=0 GO111MODULE=on go build -ldflags $(LD_FLAGS) -o $(PROJDIR)/bin/stolonctl $(REPO_PATH)/cmd/stolonctl

.PHONY: docker
docker:
	if [ -z $${TAG} ]; then echo 'TAG is undefined'; exit 1; fi; \
	docker build --build-arg PGVERSION=${PGVERSION} -t ${TAG} -f examples/kubernetes/image/docker/Dockerfile .

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: test
test:
	go test $$(go list ./... | grep -v github.com/pgvillage-tools/stolon/tests) -coverprofile cover.out -coverpkg=./...

.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@latest

.PHONY: check-coverage
check-coverage: install-go-test-coverage test
	${GOBIN}/go-test-coverage --config=./.testcoverage.yaml

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: build-images
build-images: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t keeper-$(PGVERSION) --build-arg PGVERSION=$(PGVERSION) -f Dockerfile.keeper .
	$(CONTAINER_TOOL) build -t stolonctl -f Dockerfile.stolonctl .
	$(CONTAINER_TOOL) build -t proxy -f Dockerfile.proxy .
	$(CONTAINER_TOOL) build -t sentinel -f Dockerfile.sentinel .

.PHONY: clean-e2e-containers
clean-e2e-containers: ## Clean containers for previous e2e runs
	$(CONTAINER_TOOL) ps -a | grep -v CONTAINER | sed 's/.* //' | xargs $(CONTAINER_TOOL) stop
	$(CONTAINER_TOOL) ps -a | grep -v CONTAINER | sed 's/.* //' | xargs $(CONTAINER_TOOL) rm

.PHONY: clean-e2e-images
clean-e2e-images: ## Clean containers for previous e2e runs
	$(CONTAINER_TOOL) images | awk '{if (length($$1)==54)print $$1":"$$2}' | xargs $(CONTAINER_TOOL) rmi
	$(CONTAINER_TOOL) images | awk '/<none>/{print $$3}' | xargs $(CONTAINER_TOOL) rmi

.PHONY: fast-e2e-test
fast-e2e-test:
	cd ./tests/etcdv3 && PGVERSION=$(PGVERSION) go test -count=1 -v ./...

.PHONY: full-e2e-test
full-e2e-test: build-images clean-e2e-containers clean-images fast-e2e-test

.PHONY: e2e-test
e2e-test: fast-e2e-test
