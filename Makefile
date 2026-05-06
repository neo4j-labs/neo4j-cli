GOPATH := $(shell go env GOPATH)
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint

.PHONY: build build-neo4j snapshot test lint fmt fmt-check license-check run-neo4j clean changelog generate generate-check npm-publish-dry npm-bootstrap

## build: build neo4j-cli into bin/
build: build-neo4j

## build-neo4j: build the neo4j-cli binary into bin/
build-neo4j:
	go build -o bin/neo4j-cli ./neo4j-cli

## snapshot: release build for current platform only (uses goreleaser, outputs to bin/)
snapshot:
	@mkdir -p bin/
	GORELEASER_CURRENT_TAG=dev goreleaser build --snapshot --single-target --clean
	cp dist/neo4j-cli_*/neo4j-cli bin/neo4j-cli

## test: run all tests
test:
	go test ./...

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## fmt: format all Go source files
fmt:
	go fmt ./...

## fmt-check: fail if any Go source file needs gofmt (catches drift without rewriting)
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "ERROR: the following files need gofmt:"; \
		echo "$$unformatted"; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi

## license-check: verify all .go files carry the Neo4j copyright header
## NOTE: this target requires a Unix shell (bash/sh) and the `find` + `xargs` utilities.
##       It will not work on Windows without WSL or Git Bash.
license-check:
	$(GOPATH)/bin/addlicense -f ./addlicense -check $$(find . -name "*.go" -type f -print0 | xargs -0)

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

## generate: run all `go generate` directives (regenerates per-binary skill bundles)
generate:
	go generate ./...

## generate-check: regenerate and fail if the working tree drifts (CI gate)
generate-check: generate
	@if ! git diff --exit-code; then \
		echo ""; \
		echo "ERROR: generated files are stale. Run 'make generate' and commit the result."; \
		exit 1; \
	fi

## npm-publish-dry: dry-run the npm publish flow (renders templates, exercises ordering)
## REQUIRES: `make snapshot` first if you want real binaries copied into the rendered packages.
##           snapshot is NOT a prerequisite here — it is slow and re-running it on every
##           dry-run would be painful. In --dry-run mode publish.sh tolerates missing binaries
##           by stubbing them with a 1-byte placeholder, so this target works against an empty
##           dist/ and is purely a template-rendering + publish-ordering sanity check.
npm-publish-dry:
	GORELEASER_CURRENT_TAG=v0.0.0-dry distribution/npm/publish.sh --dry-run

## npm-bootstrap: one-time helper to claim the 9 @neo4j-labs/* package names on the registry
## REQUIRES: `npm login --scope=@neo4j-labs` first; the script aborts on an unauthenticated
##           session. Publishes 1-byte stubs at version 0.0.0-bootstrap.1 under dist-tag
##           `bootstrap` so unqualified `npm i` never resolves to one of them. Idempotent —
##           re-running after a partial failure skips already-published packages.
npm-bootstrap:
	bash distribution/npm/bootstrap-stubs.sh
