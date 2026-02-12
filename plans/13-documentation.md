# Plan 13 — Documentation & Operational Readiness

## Summary

Complete all documentation, operational tooling, and polish needed to ship lil-olt-metrics as a usable product: README, usage guide, configuration reference, Dockerfile, CI pipeline, and any remaining `.context.md` updates. After this plan, a new user can clone the repo, understand the project, build it, run it, and deploy it.

## Dependencies

- **All plans 01–12** — Documentation describes the finished product

## Scope

### In Scope

- `README.md`:
  - Project overview and goals (single-binary OTLP → Prometheus bridge)
  - Quick start (build, run with defaults, send test data, query)
  - Configuration summary with link to full reference
  - Architecture overview with diagram
  - Supported OTLP features and Prometheus API endpoints
  - Development setup (build, test, lint)
- Configuration reference (`docs/config-reference.md`):
  - Every config field with type, default, description, and env var override name
  - Example YAML files for common scenarios (dev, edge, remote backend)
- `Dockerfile`:
  - Multi-stage build: builder stage compiles the binary, runtime stage is scratch or distroless
  - Inject version via build arg
  - Expose default ports (4317, 4318, 9090)
  - Default command runs the server
- `docker-compose.yml` for local development:
  - lil-olt-metrics service
  - Optional Grafana pre-configured with the Prometheus datasource pointed at the query API
  - Optional OpenTelemetry Collector or telemetrygen for generating test data
- CI pipeline (GitHub Actions):
  - `ci.yml`: lint, test, build on push/PR
  - `release.yml`: build binaries for linux/darwin/amd64/arm64, create GitHub release with artifacts
- Updated `.context.md` files: ensure every directory's context file reflects the final state
- `CHANGELOG.md` with initial release entry
- Godoc comments on all exported types and functions (verify coverage)

### Out of Scope

- Hosted documentation site (GitHub pages, etc.)
- Kubernetes manifests or Helm charts
- Monitoring/alerting runbooks (beyond basic operational notes)
- Marketing or promotional materials

## Acceptance Criteria

1. `README.md` exists and a new developer can follow it to build and run the project from scratch
2. Configuration reference documents every config field with its default and env var name
3. `docker build .` produces a working container image
4. `docker-compose up` starts the server with optional Grafana
5. CI pipeline runs lint, test, and build on every push
6. Release pipeline produces binaries for at least linux/amd64 and darwin/arm64
7. All `.context.md` files are up to date with the final directory contents
8. All exported Go types and functions have godoc comments
9. `CHANGELOG.md` documents the initial release

## Key Decisions

- **Distroless runtime image**: Minimal attack surface, no shell — appropriate for a single-binary Go server
- **GitHub Actions over other CI**: Repo is on GitHub; native integration is simplest
- **docker-compose for dev ergonomics**: Lets users spin up the full stack (server + Grafana + data generator) with one command
- **Config reference as generated doc**: Consider generating the config reference from the Go struct tags to keep it in sync; if too complex, maintain manually
- **Changelog from day one**: Even for an initial release, a CHANGELOG establishes the practice for future releases
