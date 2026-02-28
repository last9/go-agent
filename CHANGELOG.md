# Changelog

All notable changes to the Last9 Go Agent will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`database.ParseDSNAttributes(dsn, driver)`** — public helper that parses a DSN and returns OTel semantic convention attributes (`server.address`, `server.port`, `db.user`, `db.name`). Useful for stamping connection attributes onto manually created wrapper spans, where `database.Open()` auto-attributes (injected via otelsql) are not inherited. Supports `postgres`, `pgx`, `mysql`, `sqlite`, `sqlite3`, and generic URL-style DSNs.

## [0.1.0] - 2025-01-01

### Added

#### Core Agent
- One-line initialization with `agent.Start()` replacing 80-150 lines of OpenTelemetry setup
- Environment-based configuration using standard OpenTelemetry variables
- Automatic resource detection (host, OS, container, process info)
- Graceful shutdown with `agent.Shutdown()`

#### Web Framework Instrumentation
- **net/http** - Standard library support with multiple patterns:
  - `NewServeMux()` for instrumented ServeMux
  - `WrapHandler()` for wrapping existing handlers
  - `Handler()`/`HandlerFunc()` for individual handlers
  - `ListenAndServe()`/`ListenAndServeTLS()` drop-in replacements
  - Context propagation helpers (`ExtractContext`, `InjectContext`)
- **Gin** - Drop-in replacements `Default()`, `New()`, and `Middleware()`
- **Chi** - `New()` instrumented router and `Use()` for existing routers
- **Echo** - `New()` instrumented Echo instance
- **Gorilla Mux** - `NewRouter()` instrumented router
- **gRPC-Gateway** - Full HTTP/gRPC gateway instrumentation:
  - `NewGrpcServer()` for instrumented gRPC servers
  - `NewGatewayMux()` for instrumented gateway mux
  - `NewDialOption()` for instrumented client connections
  - `WrapHTTPMux()` for HTTP wrapper instrumentation

#### Database Instrumentation
- PostgreSQL, MySQL, SQLite support via `database.Open()`
- Automatic query tracing with duration, rows affected
- Connection pool metrics (usage, idle, max, wait/use/idle times)
- `MustOpen()` convenience function for quick initialization

#### Redis Instrumentation
- Drop-in replacement `NewClient()` for redis.NewClient()
- Cluster support with `NewClusterClient()`
- Automatic command tracing
- Connection pool metrics following OTel semantic conventions

#### Kafka Instrumentation
- **Producer**: `NewSyncProducer()` with automatic trace context propagation
- **Consumer**: `NewConsumerGroup()` with `WrapConsumerGroupHandler()`
- Automatic metrics: messages sent/received, errors, latencies, message sizes

#### HTTP Client Instrumentation
- `NewClient()` for instrumented HTTP client
- Proper trace nesting with external API calls
- Request/response tracing

#### Metrics Support
- **Automatic Runtime Metrics**:
  - Go 1.24+: Full OTel runtime instrumentation (15+ metrics)
  - Go 1.22-1.23: Basic runtime metrics (memory, goroutines, GC)
- **Custom Metrics Helpers**:
  - `NewCounter()` - Monotonically increasing values
  - `NewHistogram()` / `NewFloatHistogram()` - Distribution of values
  - `NewGauge()` - Current value with async callback
  - `NewUpDownCounter()` - Values that can increase or decrease

#### Testing Infrastructure
- Integration tests for all instrumentations
- Test utilities (mock OTLP collector, span assertions, context helpers)
- Docker Compose setup for test services (Kafka, PostgreSQL, MySQL, Redis)

### Technical Details
- **Go Version**: Minimum 1.22, recommended 1.24+
- **OpenTelemetry**: Tracing/Metrics API 1.39.0, Semantic Conventions v1.26.0
- **OTLP Protocol**: HTTP for traces, gRPC for metrics

[0.1.0]: https://github.com/last9/go-agent/releases/tag/v0.1.0
