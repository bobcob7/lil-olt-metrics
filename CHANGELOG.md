# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.1.0] - 2026-02-12

### Added

- OTLP metrics ingestion via gRPC (port 4317) and HTTP (port 4318)
- Protobuf and JSON content type support for HTTP ingestion
- Prometheus-compatible HTTP query API (port 9090) with PromQL engine
- Instant query, range query, series, labels, label values, metadata, and build info endpoints
- OTLP-to-Prometheus metric translation with configurable label mapping
- Support for all OTLP metric types: Gauge, Sum, Histogram, ExponentialHistogram, Summary
- Delta-to-cumulative conversion with TTL eviction
- ExponentialHistogram to classic bucket conversion
- Schema URL recording as `__schema_url__` label
- Built-in filesystem storage engine with WAL and block compaction
- Prometheus remote write/read storage backend
- VictoriaMetrics import/export storage backend
- YAML configuration with environment variable overrides (prefix: `LOM_`)
- Data retention by age and size
- Dockerfile with multi-stage distroless build
- docker-compose setup with Grafana integration
- CI/CD pipelines (lint, test, build, release)
- Comprehensive integration test suite
