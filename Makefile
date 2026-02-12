GOBIN := $(CURDIR)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: build test lint fmt generate vet

build:
	go build -o $(GOBIN)/lil-olt-metrics ./cmd/server

test:
	go test ./...

lint: $(GOBIN)/golangci-lint
	$(GOBIN)/golangci-lint run ./...

fmt: $(GOBIN)/gofumpt
	$(GOBIN)/gofumpt -w .

generate: $(GOBIN)/moq
	go generate ./...

vet:
	go vet ./...

$(GOBIN)/golangci-lint: internal/tools/tools.go go.mod
	GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint

$(GOBIN)/gofumpt: internal/tools/tools.go go.mod
	GOBIN=$(GOBIN) go install mvdan.cc/gofumpt

$(GOBIN)/moq: internal/tools/tools.go go.mod
	GOBIN=$(GOBIN) go install github.com/matryer/moq
