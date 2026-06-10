# Changelog

All notable changes to the Last9 Go Agent are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Distro fingerprint** — `agent.Start()` now stamps `telemetry.distro.name=last9-go-agent` and `telemetry.distro.version` (resolved from build info, falling back to `dev`) onto the OTel resource. This lets the backend distinguish telemetry produced by this agent from a raw OpenTelemetry Go setup and track which agent version each service runs.

## [0.4.0] - 2026-06-10

### Added
- **`instrumentation/codeattr`** — span processor registered automatically by `agent.Start()` that stamps `code.function`, `code.filepath`, and `code.lineno` onto every Client, Producer, and Consumer span by walking the call stack at span creation. Server and Internal spans are skipped. Reuses its frame buffer via `sync.Pool` with no per-span heap allocation; keys use `semconv` v1.25.0 constants.
- **gRPC-Gateway path exclusion** — `grpcgateway.WrapHTTPMux` now automatically excludes high-volume infrastructure paths from tracing: `/health`, `/healthz`, `/readyz`, `/livez`, `/metrics` (exact match) and `/actuator/`, `/eureka/apps/` (prefix match). User-configured `LAST9_EXCLUDED_PATHS` applies on top.
- **`instrumentation/httpcapture`** — framework-agnostic `net/http` middleware that records request and response bodies onto the active OTel span as `http.request.body` and `http.response.body`. Opt-in via `LAST9_BODY_CAPTURE_ENABLED=true`. Configurable max bytes, error-only mode, and content-type filtering.

### Fixed
- `testutil.AssertSpanNoError` now checks `codes.Error` status code in addition to the error message, catching spans that set status but omit a message.

## [0.3.1] - 2026-05-10

### Changed
- Bumped `go.opentelemetry.io/otel` and related packages from 1.40.0 to 1.41.0.

### Documentation
- Documented GORM v2 via the upstream `gorm.io/plugin/opentelemetry` tracing plugin. `go-agent` does not wrap it — the upstream plugin is maintained by the GORM team and ships current OTel semantic conventions.

## [0.3.0] - 2026-04-07

### Added
- **`instrumentation/grpc`** — drop-in `grpcagent.NewServer()` and `grpcagent.NewClientDialOption()` for automatic OTel tracing on all unary and streaming RPCs.
- **`instrumentation/fasthttp`** — `fasthttpagent.Middleware()` with native OTel propagation; `fasthttpagent.ContextFromRequest()` for creating child spans inside handlers.
- **`instrumentation/iris`** — `irisagent.New()` drop-in and `irisagent.Middleware()` for Iris v12, with parametric route template span names.
- **gRPC-Gateway** — full HTTP/gRPC gateway instrumentation via `grpcgateway.NewGrpcServer()`, `grpcgateway.NewGatewayMux()`, `grpcgateway.NewDialOption()`, and `grpcgateway.WrapHTTPMux()`.

## [0.2.0] - 2026-04-07

### Added
- **`integrations/database`** — `database.Open()` / `database.MustOpen()` wrapping `sql.Open` with automatic query tracing, connection-pool metrics, and DSN attribute extraction. Supported drivers: `postgres`, `pgx`, `mysql`, `sqlite`, `sqlite3`.
- **`integrations/database.ParseDSNAttributes`** — public helper to stamp host, port, user, and database name onto manually created repository spans.
- **`integrations/mongodb`** — `mongoagent.NewClient()` and `mongoagent.Instrument()` tracing all CRUD, aggregation, and index operations; skips connection housekeeping noise. Includes `db.statement` span attribute.
- **`integrations/http`** — `httpagent.NewClient()` instrumented HTTP client with proper trace nesting.
- **`instrumentation/beego`** — `beegoagent.New()` and `beegoagent.Middleware()` for Beego v2.
- **`instrumentation/slog`** — `slogagent.SetDefault()`, `slogagent.NewHandler()`, and `slogagent.NewJSONHandler()` for log-trace correlation via `log/slog`. Injects `trace_id` and `span_id` on `*Context` calls.
- **`instrumentation/zap`** — `zapagent.TraceFields()` spread helper and `zapagent.New()` context-aware logger wrapper for Uber zap log-trace correlation.
- **Route exclusion** — `LAST9_EXCLUDED_PATHS`, `LAST9_EXCLUDED_PATH_PREFIXES`, and `LAST9_EXCLUDED_PATH_PATTERNS` environment variables to filter health-check and infrastructure endpoints from tracing. Works across all supported frameworks.
- **`LAST9_TRACE_SAMPLE_RATE`** — environment variable for probabilistic sampling without a custom `OTEL_TRACES_SAMPLER` config.
- **Functional options** — programmatic configuration via `agent.WithEndpoint()`, `agent.WithHeaders()`, and related option functions as an alternative to environment variables.
- **Context helpers** — `agent.ContextWithSpan`, `agent.SpanFromContext`, and related utilities for manual span management.
- **EC2 resource detection** — automatic detection of EC2 instance metadata via IMDSv2.
- **AWS SDK v2 instrumentation** — automatic tracing of AWS SDK v2 calls.

### Changed
- Bumped `go.opentelemetry.io/otel/sdk` from 1.27.0 to 1.40.0.

## [0.1.0] - 2025-12-30

### Added

#### Core Agent
- One-line initialization with `agent.Start()` replacing 80–150 lines of OpenTelemetry setup.
- Environment-based configuration using standard OpenTelemetry variables (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_SERVICE_NAME`, etc.).
- Automatic resource detection: host, OS, architecture, container ID, process info.
- Graceful shutdown with `agent.Shutdown()`.

#### Web Frameworks
- **net/http** — `NewServeMux()`, `WrapHandler()`, `Handler()`, `HandlerFunc()`, `ListenAndServe()`, `ListenAndServeTLS()`, and context propagation helpers.
- **Gin** — `Default()`, `New()`, and `Middleware()` drop-ins.
- **Chi** — `New()` instrumented router and `Use()` for existing routers.
- **Echo** — `New()` instrumented instance.
- **Gorilla Mux** — `NewRouter()` instrumented router.

#### Storage
- **SQL** — `database.Open()` / `database.MustOpen()` for PostgreSQL, MySQL, and SQLite.
- **Redis** — `redisagent.NewClient()` and `redisagent.NewClusterClient()` with automatic command tracing and pool metrics.
- **Kafka** — `kafkaagent.NewSyncProducer()` and `kafkaagent.NewConsumerGroup()` with automatic trace-context propagation from producer to consumer.

#### Metrics
- **Automatic runtime metrics** — Go 1.24+: full OTel runtime suite (15+ metrics); Go 1.22–1.23: memory, goroutines, GC basics.
- **Custom metrics helpers** — `metrics.NewCounter()`, `metrics.NewHistogram()`, `metrics.NewFloatHistogram()`, `metrics.NewGauge()`, `metrics.NewUpDownCounter()`.

#### Testing
- `testutil` package: mock OTLP collector, span assertions, context helpers.
- Docker Compose setup for integration tests (Postgres, MySQL, Redis, Kafka).

[Unreleased]: https://github.com/last9/go-agent/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/last9/go-agent/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/last9/go-agent/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/last9/go-agent/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/last9/go-agent/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/last9/go-agent/releases/tag/v0.1.0
