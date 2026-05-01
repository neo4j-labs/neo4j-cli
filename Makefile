GOPATH := $(shell go env GOPATH)
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint

.PHONY: build build-aura build-neo4j snapshot test lint fmt license-check run-aura run-neo4j clean changelog

## build: build both aura-cli and neo4j-cli into bin/
build: build-aura build-neo4j

## build-aura: build the standalone aura-cli binary into bin/
build-aura:
	go build -o bin/aura-cli ./neo4j-cli/aura/cmd

## build-neo4j: build the neo4j-cli binary into bin/
build-neo4j:
	go build -o bin/neo4j-cli ./neo4j-cli

## snapshot: release build for current platform only (uses goreleaser, outputs to bin/)
snapshot:
	@mkdir -p bin/
	GORELEASER_CURRENT_TAG=dev AURA_CLI_VERSION=dev goreleaser build --snapshot --single-target --clean
	cp dist/neo4j-cli_*/neo4j-cli bin/neo4j-cli
	cp dist/aura-cli_*/aura-cli bin/aura-cli

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

## changelog: add a new changelog entry
changelog:
	changie new
