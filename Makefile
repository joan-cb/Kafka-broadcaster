BINARY     := broadcaster
UI_BINARY  := kafkaui
BUILD_DIR  := bin
CMD        := ./cmd/broadcaster
UI_CMD     := ./cmd/kafkaui
MODULE     := github.com/globalcommerce/kafka-broadcaster

# Recipes use /bin/sh; interactive-only PATH (e.g. only in ~/.bashrc) may omit Go.
# Append usual install locations so tarball/SDK installs are found.
export PATH := $(PATH):/usr/local/go/bin:$(HOME)/go/bin

# $(shell ...) does not inherit export PATH from this Makefile; pass PATH explicitly.
ifeq ($(shell PATH="$(PATH)" command -v go 2>/dev/null),)
$(error go not found. Install: sudo apt install golang-go, or https://go.dev/dl/ — tarball installs use /usr/local/go/bin; ensure that directory exists or add Go bin to PATH in ~/.profile and log in again.)
endif

.PHONY: all build build/ui run run/ui lint fmt vet tidy \
        test test/all test/unit test/stubbed test/functional test/e2e test/nft \
        clean help

all: build

## build: Compile the broadcaster binary into ./bin/
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD)

## build/ui: Compile the Kafka dev UI binary into ./bin/
build/ui:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(UI_BINARY) $(UI_CMD)

## run: Run the broadcaster using CONFIG_PATH (default: config/config.example.yaml)
run:
	CONFIG_PATH?=config/config.example.yaml go run $(CMD)

## run/ui: Run the Kafka dev UI (KAFKA_BROKERS defaults to localhost:9092, UI_PORT to 8080)
run/ui:
	go run $(UI_CMD)

## fmt: Format all Go source files
fmt:
	gofmt -w ./...

## vet: Run go vet on all packages
vet:
	go vet ./...

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## tidy: Tidy and verify go.mod / go.sum
tidy:
	go mod tidy
	go mod verify

## test: Run default suites (unit + stubbed only; no Docker)
test: test/unit test/stubbed

## test/all: Run every suite: unit, stubbed, functional, e2e, nft (requires Docker; e2e runs build)
test/all: test/unit test/stubbed test/functional test/e2e test/nft

## test/unit: Run unit tests for all internal packages
test/unit:
	go test -v -race -count=1 ./internal/...

## test/stubbed: Run stubbed pipeline tests (no Kafka required)
test/stubbed:
	go test -v -race -count=1 -tags=stubbed ./tests/stubbed/...

## test/functional: Run functional tests (requires Docker for Kafka; first run may pull a large image)
test/functional:
	go test -v -race -count=1 -tags=functional -timeout=10m ./tests/functional/...

## test/e2e: Run end-to-end tests (requires Docker + binary build)
test/e2e: build
	go test -v -count=1 -tags=e2e -timeout=10m ./tests/e2e/...

## test/nft: Run non-functional throughput and benchmark tests
test/nft:
	go test -v -count=1 -tags=nft -bench=. -benchmem -timeout=120s ./tests/nft/...

## clean: Remove build artefacts
clean:
	rm -rf $(BUILD_DIR) bin/kafkaui

## help: Print this help message
help:
	@grep -E '^## ' Makefile | sed 's/## //'
