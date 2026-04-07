# Last9 Go Agent

<p>
  <a href="https://pkg.go.dev/github.com/last9/go-agent"><img src="https://pkg.go.dev/badge/github.com/last9/go-agent.svg" alt="Go Reference"></a>
  <a href="https://github.com/last9/go-agent/releases"><img src="https://img.shields.io/github/v/release/last9/go-agent" alt="Release"></a>
  <a href="https://github.com/last9/go-agent/actions/workflows/ci.yml"><img src="https://github.com/last9/go-agent/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

<p>
Last9 Go Agent gives you full observability — distributed traces, runtime metrics, and log-trace correlation — without writing the plumbing yourself. One call to <code>agent.Start()</code> replaces hundreds of lines of OpenTelemetry setup. Every framework integration is a drop-in replacement, not a wrapper you have to bolt on.
</p>

<p>
This is the SDK path: works anywhere Go runs — VMs, bare metal, Lambda, local development. If you're on Kubernetes and want zero-code instrumentation, the <a href="https://github.com/last9/last9-k8s-observability-installer">eBPF operator</a> is the right tool. Both can coexist: eBPF for base HTTP and DB coverage, this SDK for custom business spans.
</p>

## Table of Contents

- [Quick Start](#quick-start)
- [Framework Support](#framework-support)
- [Database Support](#database-support)
- [MongoDB](#mongodb)
- [Redis](#redis)
- [Kafka](#kafka)
- [HTTP Client](#http-client)
- [Log-Trace Correlation](#log-trace-correlation)
- [Metrics](#metrics)
- [Route Exclusion](#route-exclusion)
- [Configuration](#configuration)
- [Testing](#testing)

## Quick Start

```bash
go get github.com/last9/go-agent
```

Set your environment variables:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="<your last9 otlp endpoint>"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <your last9 token>"
export OTEL_SERVICE_NAME="my-service"
```

Add two lines to `main.go`:

```go
func main() {
    agent.Start()
    defer agent.Shutdown()

    // your application code, unchanged
}
```

That's it. Traces and metrics start flowing.

## Framework Support

<p>
Every web framework integration is a drop-in replacement for the standard constructor. You change the import and the instantiation call — nothing else in your application changes.
</p>

### net/http

```go
import nethttpagent "github.com/last9/go-agent/instrumentation/nethttp"

mux := nethttpagent.NewServeMux()
mux.HandleFunc("/users", usersHandler)
http.ListenAndServe(":8080", mux)

// Or wrap an existing handler
http.ListenAndServe(":8080", nethttpagent.WrapHandler(existingMux))

// Or wrap individual handlers
http.Handle("/ping", nethttpagent.Handler(pingHandler, "/ping"))

// Or use the drop-in ListenAndServe
nethttpagent.ListenAndServe(":8080", mux)
```

### Gin

```go
import ginagent "github.com/last9/go-agent/instrumentation/gin"

r := ginagent.Default()   // includes logging & recovery
r := ginagent.New()       // minimal

// Or add to an existing router
r := gin.New()
r.Use(ginagent.Middleware())
```

### Chi

```go
import chiagent "github.com/last9/go-agent/instrumentation/chi"

r := chiagent.New()

// Or instrument an existing router — add AFTER defining routes
// so the middleware can capture the matched route pattern
r := chi.NewRouter()
r.Get("/users/{id}", handler)
chiagent.Use(r)
```

### Echo

```go
import echoagent "github.com/last9/go-agent/instrumentation/echo"

e := echoagent.New()
```

### Gorilla Mux

```go
import gorillaagent "github.com/last9/go-agent/instrumentation/gorilla"

r := gorillaagent.NewRouter()
r.HandleFunc("/ping", handler).Methods("GET")
```

### gRPC

```go
import grpcagent "github.com/last9/go-agent/instrumentation/grpc"

// Server
lis, _ := net.Listen("tcp", ":50051")
s := grpcagent.NewServer()
pb.RegisterGreeterServer(s, &server{})
s.Serve(lis)

// Client
conn, _ := grpc.NewClient("localhost:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpcagent.NewClientDialOption(),
)
```

### gRPC-Gateway

```go
import (
    "github.com/last9/go-agent"
    "github.com/last9/go-agent/instrumentation/grpcgateway"
)

grpcServer := grpcgateway.NewGrpcServer()
pb.RegisterYourServiceServer(grpcServer, &server{})

gwMux := grpcgateway.NewGatewayMux()

conn, _ := grpc.NewClient("localhost:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpcgateway.NewDialOption(),
)

httpMux := http.NewServeMux()
httpMux.Handle("/", gwMux)
http.ListenAndServe(":8080", grpcgateway.WrapHTTPMux(httpMux, "my-gateway"))
```

### fasthttp

```go
import fasthttpagent "github.com/last9/go-agent/instrumentation/fasthttp"

handler := func(ctx *fasthttp.RequestCtx) {
    ctx.WriteString("hello")
}
fasthttp.ListenAndServe(":8080", fasthttpagent.Middleware(handler))

// Access the active span context inside a handler
func myHandler(ctx *fasthttp.RequestCtx) {
    otelCtx := fasthttpagent.ContextFromRequest(ctx)
    _, span := otel.Tracer("my-service").Start(otelCtx, "my-op")
    defer span.End()
}
```

### Iris

```go
import irisagent "github.com/last9/go-agent/instrumentation/iris"

app := irisagent.New()   // drop-in for iris.New()
app.Get("/ping", func(ctx iris.Context) {
    ctx.WriteString("pong")
})
app.Listen(":8080")

// Or add to an existing application
app := iris.New()
app.Use(irisagent.Middleware())
```

### Beego

```go
import beegoagent "github.com/last9/go-agent/instrumentation/beego"

app := beegoagent.New()
app.Get("/ping", func(ctx *context.Context) {
    ctx.Output.Body([]byte("pong"))
})
app.Run()

// Or add to an existing server
app := web.NewHttpSever()
app.InsertFilterChain("/*", beegoagent.Middleware())
```

## Database Support

<p>
SQL tracing uses <code>database.Open()</code> instead of <code>sql.Open()</code>. Every query gets a span. Connection pool metrics are collected automatically. The agent extracts host, port, user, and database name from your DSN and stamps them onto spans as OTel semantic convention attributes.
</p>

```go
import "github.com/last9/go-agent/integrations/database"

db, err := database.Open(database.Config{
    DriverName:   "postgres",
    DSN:          "postgres://user:pass@localhost/mydb",
    DatabaseName: "mydb",
})
defer db.Close()

// Use db normally — all queries are automatically traced
rows, err := db.Query("SELECT * FROM users")
```

```go
// Panic on error variant for quick initialization
db := database.MustOpen(database.Config{
    DriverName:   "postgres",
    DSN:          os.Getenv("DATABASE_URL"),
    DatabaseName: "mydb",
})
```

Supported drivers: `postgres`, `pgx`, `mysql`, `sqlite`, `sqlite3`.

### Manual Wrapper Spans

<p>
When you create spans around repository methods, they won't inherit the connection attributes auto-generated by the SQL layer. Use <code>ParseDSNAttributes</code> to stamp them yourself:
</p>

```go
import "github.com/last9/go-agent/integrations/database"

func (r *UserRepo) FindByID(ctx context.Context, id int) (*User, error) {
    ctx, span := tracer.Start(ctx, "FindByID")
    defer span.End()

    span.SetAttributes(database.ParseDSNAttributes(r.dsn, "mysql")...)

    // ... run query
}
```

## MongoDB

```go
import mongoagent "github.com/last9/go-agent/integrations/mongodb"

client, err := mongoagent.NewClient(mongoagent.Config{
    URI: "mongodb://localhost:27017/mydb",
})
defer client.Disconnect(context.Background())

col := client.Database("mydb").Collection("users")
col.InsertOne(ctx, bson.M{"name": "Alice"})
```

Or instrument an existing options struct:

```go
opts := options.Client().ApplyURI(os.Getenv("MONGO_URI"))
client, err := mongoagent.Instrument(opts)
```

All CRUD operations, aggregation pipelines, and index operations are traced. Connection housekeeping (`hello`, `ping`, `isMaster`) and auth handshakes are silently skipped.

## Redis

```go
import redisagent "github.com/last9/go-agent/integrations/redis"

rdb := redisagent.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// All commands are automatically traced
err := rdb.Set(ctx, "key", "value", 0).Err()
val, err := rdb.Get(ctx, "key").Result()
```

```go
// Cluster support
rdb := redisagent.NewClusterClient(&redis.ClusterOptions{
    Addrs: []string{":7000", ":7001", ":7002"},
})
```

## Kafka

### Producer

```go
import kafkaagent "github.com/last9/go-agent/integrations/kafka"

producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})
defer producer.Close()

partition, offset, err := producer.SendMessage(ctx, &sarama.ProducerMessage{
    Topic: "my-topic",
    Value: sarama.StringEncoder("Hello Kafka"),
})
```

### Consumer Group

```go
consumer, err := kafkaagent.NewConsumerGroup(kafkaagent.ConsumerConfig{
    Brokers: []string{"localhost:9092"},
    GroupID: "my-consumer-group",
})
defer consumer.Close()

handler := kafkaagent.WrapConsumerGroupHandler(&MyHandler{})
consumer.Consume(ctx, []string{"my-topic"}, handler)
```

<p>
Trace context is propagated from producer to consumer automatically. When you receive a message, its context already carries the producer's span as parent.
</p>

## HTTP Client

```go
import (
    "net/http/httptrace"
    httpagent "github.com/last9/go-agent/integrations/http"
    "go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
)

client := httpagent.NewClient(&http.Client{
    Timeout: 10 * time.Second,
})

ctx = httptrace.WithClientTrace(ctx, otelhttptrace.NewClientTrace(ctx))
req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/data", nil)
resp, err := client.Do(req)
```

## Log-Trace Correlation

<p>
The agent injects <code>trace_id</code> and <code>span_id</code> into your log entries so you can jump from a log line directly to its trace. Works with both <code>log/slog</code> and Uber's <code>zap</code>.
</p>

### slog

```go
import slogagent "github.com/last9/go-agent/instrumentation/slog"

// One-line global setup
slogagent.SetDefault(os.Stdout, nil, nil)

// All *Context calls now include trace_id and span_id
slog.InfoContext(ctx, "processing request", "user_id", 42)
// Output: {"level":"INFO","msg":"processing request","user_id":42,"trace_id":"abc123...","span_id":"def456..."}
```

Or wrap an existing handler:

```go
base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
handler := slogagent.NewHandler(base, nil)
logger := slog.New(handler)
```

Custom attribute keys:

```go
handler := slogagent.NewJSONHandler(os.Stdout, nil, &slogagent.Options{
    TraceKey: "dd.trace_id",
    SpanKey:  "dd.span_id",
})
```

Trace fields are only injected when you use `*Context` methods (`InfoContext`, `ErrorContext`, etc.) with a context that holds an active span. Calls without context pass through unchanged.

### zap

```go
import zapagent "github.com/last9/go-agent/instrumentation/zap"

// Spread trace fields inline — no wrapper needed
logger.Info("request handled",
    zap.String("path", r.URL.Path),
    zapagent.TraceFields(ctx)...,
)
```

Or use the logger wrapper for context-aware methods:

```go
base, _ := zap.NewProduction()
logger := zapagent.New(base, nil)

logger.InfoContext(ctx, "user created", zap.String("user_id", "42"))
logger.ErrorContext(ctx, "payment failed", zap.Error(err))
```

## Metrics

<p>
Runtime, HTTP, gRPC, database, Kafka, and Redis metrics are collected automatically — no configuration required. For business metrics, the <code>metrics</code> package provides helpers for the four standard instrument types.
</p>

### Custom Metrics

```go
import "github.com/last9/go-agent/metrics"

// Counter — monotonically increasing
requestCounter := metrics.NewCounter(
    "app.requests.total",
    "Total number of requests processed",
    "{request}",
)
requestCounter.Inc(ctx, attribute.String("endpoint", "/api/users"))

// Histogram — distribution of values
latencyHistogram := metrics.NewHistogram(
    "app.processing.duration",
    "Processing duration in milliseconds",
    "ms",
)
latencyHistogram.Record(ctx, duration, attribute.String("operation", "compute"))

// Gauge — current value via async callback
workerGauge := metrics.NewGauge(
    "app.workers.active",
    "Number of active worker goroutines",
    "{worker}",
    func(ctx context.Context) int64 {
        return atomic.LoadInt64(&activeWorkers)
    },
)

// UpDownCounter — value that increases and decreases
queueSize := metrics.NewUpDownCounter(
    "app.queue.size",
    "Number of items in processing queue",
    "{item}",
)
queueSize.Add(ctx, 10, attribute.String("queue", "high-priority"))
queueSize.Add(ctx, -5, attribute.String("queue", "high-priority"))
```

Use standard UCUM units: `ms`/`s` for time, `By` for bytes, `{item}`/`{request}` for counts, `%` for percentages.

### Automatic Metrics Reference

| Source | Metrics |
|--------|---------|
| **Runtime** | heap alloc, goroutines, GC count, GC pause — Go 1.24+ gets the full OTel runtime suite (15+ metrics) |
| **HTTP/gRPC** | request duration, request/response size, active requests, RPC latency |
| **Database** | connection pool usage, idle, max, wait/use/idle times |
| **MongoDB** | operation count, error count, operation duration |
| **Kafka** | messages sent/received, errors, send/process latency, message size |
| **Redis** | pool usage, command duration, connection timeouts |

## Route Exclusion

<p>
Health checks and infrastructure endpoints are excluded from tracing by default. This works across all supported frameworks.
</p>

Default excluded paths: `/health`, `/healthz`, `/metrics`, `/ready`, `/live`, `/ping`, and glob variants like `/*/health`.

Configure via environment variables:

```bash
# Exact paths
export LAST9_EXCLUDED_PATHS="/health,/healthz,/status,/version"

# Prefix exclusions
export LAST9_EXCLUDED_PATH_PREFIXES="/internal/,/debug/"

# Glob patterns
export LAST9_EXCLUDED_PATH_PATTERNS="/*/health,/*/metrics"

# Trace everything — disable all defaults
export LAST9_EXCLUDED_PATHS=""
export LAST9_EXCLUDED_PATH_PATTERNS=""
```

Matching runs in order: exact path (O(1) map lookup) → prefix → glob. First match wins.

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Yes | Last9 OTLP endpoint |
| `OTEL_EXPORTER_OTLP_HEADERS` | Yes | Authorization header |
| `OTEL_SERVICE_NAME` | No | Service name (default: `unknown-service`) |
| `OTEL_SERVICE_VERSION` | No | Service version, e.g. git commit SHA |
| `OTEL_RESOURCE_ATTRIBUTES` | No | Additional attributes as `key=value` pairs |
| `OTEL_TRACES_SAMPLER` | No | Sampling strategy (default: `always_on`) |
| `LAST9_TRACE_SAMPLE_RATE` | No | Probabilistic sample rate, e.g. `0.1` for 10% |
| `LAST9_EXCLUDED_PATHS` | No | Exact paths excluded from tracing |
| `LAST9_EXCLUDED_PATH_PREFIXES` | No | Path prefixes excluded from tracing |
| `LAST9_EXCLUDED_PATH_PATTERNS` | No | Glob patterns excluded from tracing |

The agent automatically detects and records host info, OS, architecture, container ID, and process details as resource attributes.

## Requirements

- Go 1.22 or later (1.24+ recommended — full OTel runtime instrumentation)
- OpenTelemetry Tracing/Metrics API 1.39.0
- Semantic Conventions v1.26.0

## Testing

```bash
# Unit tests — no Docker required
make test-unit

# Integration tests — requires Docker
make docker-up
make test-integration
make docker-down
```

Integration tests require Docker for Postgres, MySQL, Redis, and Kafka. Proto files for gRPC tests are generated via `buf`.

## Contributing

Open an issue first, then fork, branch, and submit a pull request. Run `golangci-lint run --timeout=5m ./...` before pushing.

## License

Apache License 2.0. See [LICENSE](LICENSE).

---

Built on [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go) and [opentelemetry-go-contrib](https://github.com/open-telemetry/opentelemetry-go-contrib).
