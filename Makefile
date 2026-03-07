GOBIN := $(CURDIR)/bin
export PATH := $(GOBIN):$(PATH)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BRANCH  ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.branch=$(BRANCH)

.PHONY: build test lint fmt generate frontend

frontend:
	cd web && npm ci && npm run build

build: frontend
	go build -ldflags "$(LDFLAGS)" -o $(GOBIN)/lil-olt-metrics ./cmd/server

test:
	go test ./...

lint: $(GOBIN)/golangci-lint
	$(GOBIN)/golangci-lint run ./...

fmt: $(GOBIN)/gofumpt
	$(GOBIN)/gofumpt -w .

generate: $(GOBIN)/moq
	go generate ./...

$(GOBIN)/golangci-lint: internal/tools/tools.go go.mod
	GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint

$(GOBIN)/gofumpt: internal/tools/tools.go go.mod
	GOBIN=$(GOBIN) go install mvdan.cc/gofumpt

$(GOBIN)/moq: internal/tools/tools.go go.mod
	GOBIN=$(GOBIN) go install github.com/matryer/moq
