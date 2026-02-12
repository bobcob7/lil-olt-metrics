# ✅ COMPLETE — Plan 02 — Configuration Loading

## Summary

Implement the configuration subsystem: Go struct definitions matching the schema in `docs/config.md`, YAML file loading, environment variable overrides (`LOM_` prefix), sensible defaults, and validation. After this plan, any downstream package can import `internal/config` and get a fully populated, validated config struct.

## Dependencies

- **Plan 01** — Go module, dependencies, and directory skeleton must exist

## Scope

### In Scope

- Config struct hierarchy in `internal/config/` matching the schema in `docs/config.md`
- YAML file loading from a path (default `lom.yaml` in working directory)
- Environment variable overrides with `LOM_` prefix and underscore-separated nesting (e.g., `LOM_OTLP_GRPC_LISTEN` → `otlp.grpc.listen`)
- Default values for all fields so the server runs with zero configuration
- Validation: port ranges, mutually exclusive options, required fields when a feature is enabled
- A top-level `Load(path string) (*Config, error)` function
- Unit tests covering: defaults only, YAML override, env override, env takes precedence over YAML, validation errors

### Out of Scope

- Hot-reload / config watching
- CLI flags (config file path is the only flag, handled in `cmd/server/main.go`)
- Config for features not yet designed

## Acceptance Criteria

1. `config.Load("")` returns a valid config with all defaults populated
2. `config.Load("path/to/lom.yaml")` parses a YAML file and merges with defaults
3. Environment variables with `LOM_` prefix override both defaults and YAML values
4. Precedence order is: env vars > YAML file > defaults
5. Validation rejects invalid values (e.g., negative ports, unknown storage engine names) with descriptive errors
6. All storage engine configs (fs, prometheus, victoriametrics) are representable
7. Unit tests pass with `t.Parallel()`, cover happy path, each override layer, and validation edge cases
8. Zero exported types beyond what other packages need (keep surface area small)

## Key Decisions

- **Single `Load` function**: One entry point that handles defaults → YAML → env layering internally, keeping the call site in `main.go` trivial
- **Env var naming convention**: `LOM_` prefix with underscores maps naturally to nested YAML keys; document the mapping in a comment on the struct
- **Validation at load time**: Fail fast with clear messages rather than letting invalid config propagate to runtime panics
- **Unexported field defaults**: Set defaults in a `newDefaults()` function rather than relying on YAML library zero-value behavior, so defaults are explicit and testable
- **No external config libraries**: Use `gopkg.in/yaml.v3` directly plus `os.Getenv` — keeps the dependency surface small and the behavior transparent
