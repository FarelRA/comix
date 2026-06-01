BINARY=comix
BUILD_DIR=.
GO=go
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build run test clean lint tidy fmt help release cross-build install

build:
	$(GO) build -o build/$(BINARY) -ldflags="-s -w -X github.com/comix/comix/internal/cli.version=$(VERSION)" ./cmd/$(BINARY)

run: build
	./$(BINARY)

test:
	$(GO) test ./... -v -count=1

test-short:
	$(GO) test ./... -v -count=1 -short

clean:
	rm -f $(BINARY)
	rm -rf dist/
	$(GO) clean ./...

lint:
	golangci-lint run ./... --timeout 5m

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

cross-build:
	GOOS=linux   GOARCH=amd64 $(GO) build -o build/$(BINARY)-linux-amd64   -ldflags="-s -w" ./cmd/$(BINARY)
	GOOS=linux   GOARCH=arm64 $(GO) build -o build/$(BINARY)-linux-arm64   -ldflags="-s -w" ./cmd/$(BINARY)
	GOOS=darwin  GOARCH=amd64 $(GO) build -o build/$(BINARY)-darwin-amd64  -ldflags="-s -w" ./cmd/$(BINARY)
	GOOS=darwin  GOARCH=arm64 $(GO) build -o build/$(BINARY)-darwin-arm64  -ldflags="-s -w" ./cmd/$(BINARY)
	GOOS=windows GOARCH=amd64 $(GO) build -o build/$(BINARY)-windows-amd64.exe -ldflags="-s -w" ./cmd/$(BINARY)

release:
	goreleaser release --clean

release-dry:
	goreleaser release --clean --snapshot --skip-publish

install:
	$(GO) install -ldflags="-s -w -X github.com/comix/comix/internal/cli.version=$(VERSION)" ./cmd/$(BINARY)

help:
	@echo "Usage:"
	@echo "  make build          Build the Comix binary"
	@echo "  make run            Build and run the binary"
	@echo "  make test           Run all tests"
	@echo "  make test-short     Run tests without -race (faster)"
	@echo "  make clean          Remove build artifacts"
	@echo "  make lint           Run golangci-lint"
	@echo "  make vet            Run go vet"
	@echo "  make tidy           Run go mod tidy"
	@echo "  make fmt            Format Go source files"
	@echo "  make cross-build    Cross-compile for all platforms"
	@echo "  make release        Release via goreleaser"
	@echo "  make release-dry    Snapshot release (no publish)"
	@echo "  make install        Install to GOPATH/bin"
