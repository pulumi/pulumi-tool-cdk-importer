WORKING_DIR		:= $(shell pwd)
PACK             := pulumi-tool-cdk-importer

build:
	go build -o $(WORKING_DIR)/bin/$(PACK)

generate:
	go generate ./...
