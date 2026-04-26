GOCACHE ?= /tmp/nebula-mnemosina-gocache
CGO_ENABLED ?= 0
CONTAINER_TOOL ?= podman
COMPOSE ?= podman compose

.PHONY: generate
generate:
	sqlc generate

.PHONY: test
test:
	CGO_ENABLED=$(CGO_ENABLED) GOCACHE=$(GOCACHE) go test ./...

.PHONY: build
build:
	CGO_ENABLED=$(CGO_ENABLED) GOCACHE=$(GOCACHE) go build -o bin/nebula-mnemosina ./cmd/nebula-mnemosina

.PHONY: tidy
tidy:
	GOCACHE=$(GOCACHE) go mod tidy

.PHONY: image-build
image-build:
	$(CONTAINER_TOOL) build -t nebula-mnemosina:local .

.PHONY: compose-up
compose-up:
	$(COMPOSE) up --build
