GOCACHE ?= /tmp/nebula-mnemosina-gocache
CGO_ENABLED ?= 0

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
