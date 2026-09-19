TEST ?= $$(go list ./... | grep -v 'vendor')
GOFMT_FILES ?= $$(find . -name '*.go' | grep -v vendor)
HOSTNAME ?= registry.terraform.io
NAMESPACE ?= the-maldridge
NAME ?= aoss
BINARY ?= terraform-provider-${NAME}
VERSION ?= 0.1.0
OS_ARCH ?= $(shell go env GOOS)_$(shell go env GOARCH)

default: install

test:
	go test $(TEST) -timeout=30s -parallel=4

fmt:
	gofmt -w -s $(GOFMT_FILES)

vet:
	go vet ./...

docs:
	tfplugindocs generate

install:
	go install .

debug:
	go run . -debug

local:
	goreleaser local --clean

release:
	goreleaser release --clean

.PHONY: install test vet fmt docs debug local release
