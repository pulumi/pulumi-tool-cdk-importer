WORKING_DIR		:= $(shell pwd)
PACK             := pulumi-tool-cdk-importer
LOCAL_VERSION ?= 1.0.0-alpha.0+dev

build:
	go build -o $(WORKING_DIR)/bin/$(PACK)

generate:
	go generate ./...

test:
	go test -v -short -coverpkg=./... -coverprofile=coverage.txt ./...

local_package:
	goreleaser release --snapshot --clean


local_install: local_package
	pulumi plugin install tool cdk-importer $(LOCAL_VERSION) --file ./goreleaser/pulumi-tool-cdk-importer_darwin_arm64_v8.0/pulumi-tool-cdk-importer --reinstall
