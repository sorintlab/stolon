ifndef PGVERSION
    $(error PGVERSION is undefined)
endif
ifndef TAG
    $(error TAG is undefined)
endif

TEMPFILE := $(shell mktemp)

.PHONY: build

build: Dockerfile
	docker build -t ${TAG} .
	-rm Dockerfile

Dockerfile: FORCE
	sed -e "s/\$${PGVERSION}/${PGVERSION}/" Dockerfile.template > Dockerfile

FORCE:

