# ✅ COMPLETE — Plan 06 — OTLP Ingestion Handlers (gRPC + HTTP)

## Summary

Implement the gRPC and HTTP endpoints that receive OTLP `ExportMetricsServiceRequest` messages, decode them, pass them through the translator (Plan 05), and write the resulting samples to the Store (Plan 04). This is the inbound data path — the front door of the system.

## Dependencies

- **Plan 04** — In-memory Store (write target)
- **Plan 05** — OTLP-to-Prometheus Translator

## Scope

### In Scope

- gRPC handler implementing `opentelemetry.proto.collector.metrics.v1.MetricsService/Export` on the configured port (default 4317)
- HTTP handler accepting `POST /v1/metrics` with `Content-Type: application/x-protobuf` on the configured port (default 4318)
- Protobuf deserialization of `ExportMetricsServiceRequest` for both transports
- gzip decompression for both gRPC and HTTP
- Request validation: reject empty or malformed payloads with appropriate status codes
- Partial success handling: if some data points fail translation, still accept the rest and report partial success in the response
- Graceful handling of backpressure: if the Store is slow, block rather than drop (bounded by request timeout)
- Metrics: count of received requests, data points, and errors (using slog structured logging for now; Prometheus self-metrics deferred)
- Constructor accepting `Store`, translator, config, and `*slog.Logger`
- Unit tests covering: successful ingestion end-to-end (gRPC and HTTP), gzip decompression, malformed payload rejection, partial success, empty request handling

### Out of Scope

- JSON content type for HTTP (Plan 11)
- TLS configuration
- Authentication / authorization
- Rate limiting
- Server lifecycle management (Plan 08 handles listener setup and graceful shutdown)

## Acceptance Criteria

1. A valid protobuf `ExportMetricsServiceRequest` sent via gRPC is accepted, translated, and written to the Store
2. A valid protobuf request sent via HTTP POST to `/v1/metrics` is accepted, translated, and written to the Store
3. gzip-compressed requests are decompressed and processed correctly on both transports
4. Malformed protobuf payloads return gRPC `InvalidArgument` / HTTP 400
5. Empty requests return success with zero data points processed
6. Partial translation failures return partial success (successful points are still written)
7. Samples written via ingestion are readable via `Store.Select`
8. All tests use `t.Parallel()`, mock the Store and translator via moq
9. gRPC and HTTP handlers are independently constructable (can run one without the other)

## Key Decisions

- **Separate gRPC and HTTP handlers**: Each transport has its own handler struct; they share the translator and Store via constructor injection
- **Protobuf-only for MVP HTTP**: JSON content type adds complexity (different proto serialization path) and is rarely used in practice; defer to Plan 11
- **Block on backpressure rather than drop**: Losing metrics silently is worse than temporary latency; let the client's timeout be the safety valve
- **Partial success over all-or-nothing**: OTLP spec supports partial success; implement it from the start so clients get accurate feedback
- **No ConnectRPC for OTLP**: OTLP defines its own gRPC service; use standard `google.golang.org/grpc` for the OTLP gRPC endpoint (ConnectRPC is for any custom APIs if needed later)
