# ✅ COMPLETE — Plan 01 — Project Foundation & Build Infrastructure

## Summary

Bootstrap the Go module with all required dependencies, build tooling (Makefile, linting, formatting), CI configuration, and the directory skeleton. After this plan, `make build`, `make lint`, and `make test` all pass on an empty project.

## Dependencies

None — this is the first plan.

## Scope

### In Scope

- Go module (`go.mod` / `go.sum`) with all anticipated dependencies added
- `Makefile` with targets: `build`, `test`, `lint`, `fmt`, `generate`, `vet`
- `internal/tools/tools.go` with `//go:build tools` tag tracking `gofumpt`, `moq`, `golangci-lint`, and any other dev tools
- `.golangci.yml` with linting rules appropriate for the project style (no blank lines in functions, etc.)
- Directory skeleton: `cmd/server/`, `internal/{config,ingest,store,query}/`
- Placeholder `main.go` in `cmd/server/` that compiles and exits cleanly
- `.gitignore` for Go binaries, editor files, OS artifacts

### Out of Scope

- Any application logic
- CI/CD pipeline files (GitHub Actions, etc.) — deferred to plan 13
- Docker/container configuration

## Acceptance Criteria

1. `go build ./...` succeeds with zero errors
2. `go vet ./...` succeeds with zero warnings
3. `make lint` runs golangci-lint and passes
4. `make test` runs (no tests yet, but the target works)
5. `make fmt` formats all files with gofumpt
6. `make generate` runs go generate across the module
7. `internal/tools/tools.go` compiles and pins all tool versions in `go.mod`
8. All anticipated runtime dependencies (OTLP protos, gRPC, Prometheus engine/TSDB, ConnectRPC, YAML parser, slog) are in `go.mod`
9. `cmd/server/main.go` compiles and produces a binary that exits 0

## Key Decisions

- **Makefile over task runners**: `make` is ubiquitous in Go projects, keeps the build portable
- **gofumpt over gofmt**: Stricter formatting aligns with project style (no blank lines in functions)
- **Tools in go.mod**: Pin exact tool versions for reproducible builds; `tools.go` with build tag keeps them out of the binary
- **All deps up front**: Adding dependencies now avoids repeated `go mod tidy` churn in later plans; downstream plans can focus purely on application code
- **golangci-lint configuration**: Enable linters that enforce project conventions (gofumpt, revive, errcheck, govet, staticcheck); disable noisy or irrelevant linters
