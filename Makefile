SHELL := /usr/bin/env bash
.DEFAULT_GOAL := all

MAKEFLAGS += --no-print-directory

PROJECT_ROOT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY: help # Print this help message.
help:
	@grep -E '^\.PHONY: [a-zA-Z0-9_-]+ .*?# .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = "(: |#)"}; {printf "%-30s %s\n", $$2, $$3}'

.PHONY: all # Build the application.
all: build

.PHONY: build # Build the binary for the current platform.
build:
	./tools/goreleaser.sh build --snapshot --clean --single-target

.PHONY: build-all # Build binaries for all platforms.
build-all:
	./tools/goreleaser.sh build --snapshot --clean

.PHONY: release-snapshot # Create a full release snapshot (binaries, archives, packages).
release-snapshot:
	./tools/goreleaser.sh release --snapshot --clean --skip=publish

.PHONY: install # Build and install the binary to $GOPATH/bin.
install: build
	cp ./dist/qm_$(shell go env GOOS)_$(shell go env GOARCH)*/qm $(shell go env GOPATH)/bin/qm

.PHONY: run # Run the application locally.
run: build
	./dist/qm_$(shell go env GOOS)_$(shell go env GOARCH)*/qm

.PHONY: test # Run unit tests.
test:
	go test -race -covermode=atomic ./...

.PHONY: test-verbose # Run unit tests with verbose output.
test-verbose:
	go test -v -race -covermode=atomic ./...

.PHONY: lint # Lint the code.
lint:
	./tools/golangci-lint.sh run --timeout 2m30s

.PHONY: lint-fix # Lint and fix the code.
lint-fix:
	./tools/golangci-lint.sh run --fix
	go mod tidy

.PHONY: fmt # Format the code.
fmt:
	go fmt ./...

.PHONY: verify # Verify go modules are tidy.
verify:
	go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum is not tidy" && exit 1)

.PHONY: release # Tag and push the next version (auto-detected from commits).
release:
	@VERSION=$$(./tools/svu.sh next) && \
	echo "Current version: $$(./tools/svu.sh current)" && \
	echo "Next version:    $$VERSION" && \
	echo "" && \
	read -p "Proceed? [y/N] " confirm && [ "$$confirm" = "y" ] && \
	git tag -a $$VERSION -m "Release $$VERSION" && \
	git push origin $$VERSION

.PHONY: release-patch # Tag and push a patch release.
release-patch:
	@VERSION=$$(./tools/svu.sh patch) && \
	echo "Current version: $$(./tools/svu.sh current)" && \
	echo "Next version:    $$VERSION" && \
	git tag -a $$VERSION -m "Release $$VERSION" && \
	git push origin $$VERSION

.PHONY: release-minor # Tag and push a minor release.
release-minor:
	@VERSION=$$(./tools/svu.sh minor) && \
	echo "Current version: $$(./tools/svu.sh current)" && \
	echo "Next version:    $$VERSION" && \
	git tag -a $$VERSION -m "Release $$VERSION" && \
	git push origin $$VERSION

.PHONY: release-major # Tag and push a major release.
release-major:
	@VERSION=$$(./tools/svu.sh major) && \
	echo "Current version: $$(./tools/svu.sh current)" && \
	echo "Next version:    $$VERSION" && \
	git tag -a $$VERSION -m "Release $$VERSION" && \
	git push origin $$VERSION

.PHONY: version # Show current and next version.
version:
	@echo "Current: $$(./tools/svu.sh current)"
	@echo "Next:    $$(./tools/svu.sh next)"

.PHONY: deps # Download dependencies.
deps:
	go mod download
	go mod tidy