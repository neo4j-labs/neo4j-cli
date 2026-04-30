GOPATH := $(shell go env GOPATH)
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint

.PHONY: build build-aura build-neo4j test lint fmt license-check run-aura run-neo4j clean changelog changelog-aura changelog-neo4j

## build: build both aura-cli and neo4j-cli into bin/
build: build-aura build-neo4j

## build-aura: build the standalone aura-cli binary into bin/
build-aura:
	go build -o bin/aura-cli ./neo4j-cli/aura/cmd

## build-neo4j: build the neo4j-cli binary into bin/
build-neo4j:
	go build -o bin/neo4j-cli ./neo4j-cli

## test: run all tests
test:
	go test ./...

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## fmt: format all Go source files
fmt:
	go fmt ./...

## license-check: verify all .go files carry the Neo4j copyright header
## NOTE: this target requires a Unix shell (bash/sh) and the `find` + `xargs` utilities.
##       It will not work on Windows without WSL or Git Bash.
license-check:
	$(GOPATH)/bin/addlicense -f ./addlicense -check $$(find . -name "*.go" -type f -print0 | xargs -0)

## run-aura: run the standalone aura-cli without building
run-aura:
	go run ./neo4j-cli/aura/cmd

## run-neo4j: run the neo4j-cli without building
run-neo4j:
	go run ./neo4j-cli

## clean: remove the bin/ directory
clean:
	rm -rf bin/
	rm -rf dist/

## changelog: add a new changelog entry (prompts for project: aura-cli or neo4j-cli)
changelog:
	changie new

## changelog-aura: add a new changelog entry for aura-cli (skips project prompt)
changelog-aura:
	changie new --project aura-cli

## changelog-neo4j: add a new changelog entry for neo4j-cli (skips project prompt)
changelog-neo4j:
	changie new --project neo4j-cli
