.PHONY: all
all: validate test

## Run validates
.PHONY: validate
validate:
	golangci-lint run

## Run tests
.PHONY: test
test:
	go test -v -race ./...
