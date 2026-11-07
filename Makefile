# DEP-7: one entry point for the common tasks. The Dockerfile's build stage runs
# `make lint vet test build`, and CI runs the same targets, so all three agree.

SHELL    := /bin/bash
BIN      ?= bin/ddia
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BACKUPS  ?= backups
SERVICE  := ddia
COMPOSE  ?= docker compose

.PHONY: help up down test lint vet build backup

## help: list the targets
help:
	@sed -n 's/^## //p' $(MAKEFILE_LIST)

## up: build the image and start the service at http://localhost:8080 (DEP-2)
up:
	$(COMPOSE) up -d --build

## down: stop and remove the container; the ddia-data volume is kept
down:
	$(COMPOSE) down

## test: run the tests with the race detector
test:
	go test -race ./...

## lint: fail if any Go file is not gofmt-formatted
lint:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

## vet: run go vet
vet:
	go vet ./...

## build: static, cgo-free binary at $(BIN) (DEP-1)
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/ddia

## backup: stop the container, archive /data to backups/ddia-data-<UTC time>.tar.gz, start it again (DEP-8)
# Stopping first means the SQLite file and its WAL are copied in a consistent state.
# The container is started again only if it was running, and even when the copy fails;
# a failed copy is left as *.partial. The archive holds book text, so backups/ is git-ignored.
backup:
	@set -o pipefail; \
	running="$$(docker inspect -f '{{.State.Running}}' $(SERVICE) 2>/dev/null)" \
	  || { echo "no $(SERVICE) container: run 'make up' first" >&2; exit 1; }; \
	mkdir -p $(BACKUPS); \
	file="$(BACKUPS)/ddia-data-$$(date -u +%Y%m%dT%H%M%SZ).tar.gz"; \
	if [ "$$running" = true ]; then $(COMPOSE) stop $(SERVICE) || exit 1; fi; \
	docker cp $(SERVICE):/data - | gzip > "$$file.partial"; status=$$?; \
	if [ "$$running" = true ]; then $(COMPOSE) start $(SERVICE); fi; \
	if [ $$status -ne 0 ]; then echo "backup failed, partial archive at $$file.partial" >&2; exit $$status; fi; \
	mv "$$file.partial" "$$file" && echo "backup written to $$file"
