# Last9 Go Agent

A drop-in OpenTelemetry agent for Go applications that minimizes code changes while providing comprehensive observability with automatic traces and metrics.

[![Go Reference](https://pkg.go.dev/badge/github.com/last9/go-agent.svg)](https://pkg.go.dev/github.com/last9/go-agent)

## 📋 Table of Contents

- [Key Features](#-key-features)
- [Quick Start](#-sdk-quick-start-this-repo)
- [Framework Support](#-framework-support) - net/http • Gin • Chi • Echo • Gorilla • gRPC-Gateway
- [Database Support](#️-database-support) - PostgreSQL • MySQL • SQLite
- [Redis Support](#-redis-support)
- [Kafka Support](#-kafka-support) - Producers • Consumers
- [HTTP Client](#-http-client-support)
- [Metrics Support](#-metrics-support) - Automatic • Custom • Runtime
- [Configuration](#️-configuration)
- [Requirements & Compatibility](#-requirements--compatibility)
- [Testing](#-testing)
- [SDK vs eBPF](#-sdk-vs-ebpf-full-comparison)

## ✨ Key Features

- 🚀 **One-line initialization** - `agent.Start()` replaces 80-150 lines of OpenTelemetry setup code
- 🔌 **Drop-in replacements** - Minimal code changes for Gin, Echo, Gorilla, gRPC-Gateway (Chi requires wrapper)
- 🎯 **Auto-instrumentation** - HTTP, gRPC, SQL, Redis, Kafka automatically traced with proper span nesting
- 📊 **Automatic metrics** - Runtime (memory, GC, goroutines), HTTP, gRPC, database, Kafka, Redis metrics out-of-the-box
- 📈 **Custom metrics** - Simple helpers for counters, histograms, gauges for business metrics
- ⚙️ **Environment-based config** - Uses standard OpenTelemetry environment variables (no hardcoded config)
- 🔍 **Complete observability** - Full distributed tracing + metrics across all layers (HTTP → gRPC → DB → External APIs)

## 📋 Requirements & Compatibility

### Minimum Requirements
- **Go Version**: 1.22 or later (1.24+ recommended for full runtime metrics)
- **Environment**: Works on Linux, macOS, Windows
- **Docker**: Required only for integration tests

### Go Version Feature Matrix
| Go Version | Support Level | Runtime Metrics |
|------------|---------------|-----------------|
| **1.24+** | Full | Complete OTel runtime instrumentation (15+ metrics) |
| **1.22-1.23** | Full | Basic runtime metrics (memory, goroutines, GC) |
| **< 1.22** | Not supported | - |

The agent provides comprehensive telemetry including:
- **Full distributed tracing** across all instrumented frameworks
- **Automatic runtime metrics** (varies by Go version, see above)
- **Custom metrics** support for business-specific observability

### Supported Frameworks & Libraries
| Category | Supported | Version |
|----------|-----------|---------|
| **Web Frameworks** | net/http, Gin, Chi, Echo, Gorilla Mux, gRPC-Gateway | Latest stable |
| **Databases** | PostgreSQL, MySQL, SQLite | Any version |
| **Message Queues** | Kafka (IBM Sarama) | 2.6.0+ |
| **Caching** | Redis (go-redis) | v9 |
| **OpenTelemetry** | OTLP/HTTP (traces), OTLP/gRPC (metrics) | 1.39.0 |

### OpenTelemetry Specifications
This agent implements:
- **Traces**: OpenTelemetry Tracing API 1.39.0
- **Metrics**: OpenTelemetry Metrics API 1.39.0
- **Semantic Conventions**: v1.26.0
- **OTLP Protocol**: HTTP (traces), gRPC (metrics)

See [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/) for specification details.

---

The Last9 Go Agent provides:
- **Single line initialization**: `agent.Start()`
- **Drop-in replacements** for popular frameworks
- **Auto-configuration** from environment variables
- **Pre-built integrations** for databases, Redis, Kafka, and HTTP clients
- **Automatic metrics** for runtime, HTTP, gRPC, databases, Kafka, Redis
- **Custom metrics helpers** for business-specific observability

## 🎭 Two Ways to Instrument Go with Last9

Last9 offers **two approaches** for Go instrumentation - choose based on your environment:

| Approach | Environment | Code Changes | Best For |
|----------|-------------|--------------|----------|
| **SDK** (this repo) | Anywhere | Minimal (2 lines) | VMs, Lambda, local dev, fine control |
| **eBPF** ([operator](https://github.com/last9/last9-k8s-observability-installer)) | Kubernetes only | None | K8s production, zero-code, scale |

### When to Use SDK (This Approach)
- ✅ Running on VMs, bare metal, or Lambda
- ✅ Local development (no Kubernetes)
- ✅ Need custom business logic spans
- ✅ Want fine-grained control
- ✅ Privileged access not allowed

### When to Use eBPF (Operator)
- ✅ Running in Kubernetes
- ✅ Want truly zero code changes
- ✅ Standardizing across many services
- ✅ Don't need custom spans
- ✅ Security team approves eBPF

**Can use both?** Yes! Use eBPF for base instrumentation (HTTP, DB) + SDK for custom spans.

---

## 🚀 SDK Quick Start (This Repo)

### 1. Install

```bash
go get github.com/last9/go-agent
```

### 2. Configure

Set environment variables (or use a `.env` file):

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="<your last9 otel endpoint>"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <your last9 token>"
export OTEL_SERVICE_NAME="my-service"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=local,team=backend"
```

## 📚 Framework Support

### net/http (Standard Library)

For applications using Go's standard `net/http` package without any framework:

```go
import nethttpagent "github.com/last9/go-agent/instrumentation/nethttp"

func main() {
    agent.Start()
    defer agent.Shutdown()

    // Option 1: Use instrumented ServeMux (recommended)
    mux := nethttpagent.NewServeMux()
    mux.HandleFunc("/users", usersHandler)
    mux.HandleFunc("/orders", ordersHandler)
    http.ListenAndServe(":8080", mux)

    // Option 2: Wrap existing handler/mux
    stdMux := http.NewServeMux()
    stdMux.HandleFunc("/api", apiHandler)
    http.ListenAndServe(":8080", nethttpagent.WrapHandler(stdMux))

    // Option 3: Wrap individual handlers
    http.Handle("/ping", nethttpagent.Handler(pingHandler, "/ping"))
    http.ListenAndServe(":8080", nil)

    // Option 4: Drop-in replacement for ListenAndServe
    mux := http.NewServeMux()
    mux.HandleFunc("/data", dataHandler)
    nethttpagent.ListenAndServe(":8080", mux)  // Automatically wraps handler
}
```

### Gin

```go
import ginagent "github.com/last9/go-agent/instrumentation/gin"

func main() {
    agent.Start()
    defer agent.Shutdown()

    // Option 1: Use Default (includes logging & recovery)
    r := ginagent.Default()

    // Option 2: Use New (minimal setup)
    r := ginagent.New()

    // Option 3: Add to existing router
    r := gin.New()
    r.Use(ginagent.Middleware())

    r.GET("/ping", handler)
    r.Run(":8080")
}
```

### Chi

```go
import chiagent "github.com/last9/go-agent/instrumentation/chi"

func main() {
    agent.Start()
    defer agent.Shutdown()

    // Option 1: New instrumented router
    r := chiagent.New()

    // Option 2: Add to existing router (AFTER defining routes)
    r := chi.NewRouter()
    r.Get("/users/{id}", handler)
    chiagent.Use(r)  // Add AFTER routes for proper pattern capture

    http.ListenAndServe(":8080", r)
}
```

### Echo

```go
import echoagent "github.com/last9/go-agent/instrumentation/echo"

func main() {
    agent.Start()
    defer agent.Shutdown()

    // New instrumented Echo instance
    e := echoagent.New()

    e.GET("/ping", func(c echo.Context) error {
        return c.String(200, "pong")
    })
    e.Start(":8080")
}
```

### Gorilla Mux

```go
import gorillaagent "github.com/last9/go-agent/instrumentation/gorilla"

func main() {
    agent.Start()
    defer agent.Shutdown()

    // New instrumented router
    r := gorillaagent.NewRouter()

    r.HandleFunc("/ping", handler).Methods("GET")
    http.ListenAndServe(":8080", r)
}
```

### gRPC-Gateway

```go
import (
    "github.com/last9/go-agent"
    "github.com/last9/go-agent/instrumentation/grpcgateway"
)

func main() {
    agent.Start()
    defer agent.Shutdown()

    // gRPC server (auto-instrumented)
    grpcServer := grpcgateway.NewGrpcServer()
    pb.RegisterYourServiceServer(grpcServer, &server{})

    // gRPC-Gateway mux (auto-instrumented)
    gwMux := grpcgateway.NewGatewayMux()

    // gRPC client connection (auto-instrumented)
    conn, _ := grpc.NewClient("localhost:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpcgateway.NewDialOption(),
    )

    // HTTP wrapper (auto-instrumented)
    httpMux := http.NewServeMux()
    httpMux.Handle("/", gwMux)
    handler := grpcgateway.WrapHTTPMux(httpMux, "my-gateway")

    http.ListenAndServe(":8080", handler)
}
```

## 🗄️ Database Support

### PostgreSQL / MySQL / SQLite

```go
import "github.com/last9/go-agent/integrations/database"

db, err := database.Open(database.Config{
    DriverName:   "postgres",
    DSN:          "postgres://user:pass@localhost/mydb",
    DatabaseName: "mydb",
})
defer db.Close()

// Use normally - all queries are automatically traced!
rows, err := db.Query("SELECT * FROM users")
```

### Quick initialization (panics on error):

```go
db := database.MustOpen(database.Config{
    DriverName:   "postgres",
    DSN:          os.Getenv("DATABASE_URL"),
    DatabaseName: "mydb",
})
defer db.Close()
```

## 🔴 Redis Support

```go
import redisagent "github.com/last9/go-agent/integrations/redis"

// Drop-in replacement for redis.NewClient()
rdb := redisagent.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
defer rdb.Close()

// All Redis commands are automatically traced!
err := rdb.Set(ctx, "key", "value", 0).Err()
val, err := rdb.Get(ctx, "key").Result()
```

### Redis Cluster:

```go
rdb := redisagent.NewClusterClient(&redis.ClusterOptions{
    Addrs: []string{":7000", ":7001", ":7002"},
})
```

## 📨 Kafka Support

### Producer (Sync)

```go
import kafkaagent "github.com/last9/go-agent/integrations/kafka"

// Create instrumented producer
producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})
defer producer.Close()

// Send message (automatically traced with context propagation)
partition, offset, err := producer.SendMessage(ctx, &sarama.ProducerMessage{
    Topic: "my-topic",
    Value: sarama.StringEncoder("Hello Kafka"),
})
```

### Consumer (Consumer Group)

```go
import kafkaagent "github.com/last9/go-agent/integrations/kafka"

// Implement your handler
type MyHandler struct{}

func (h *MyHandler) Setup(session sarama.ConsumerGroupSession) error {
    return nil
}

func (h *MyHandler) Cleanup(session sarama.ConsumerGroupSession) error {
    return nil
}

func (h *MyHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        // Message context includes trace from producer
        ctx := session.Context()

        // Process message
        log.Printf("Message: %s", string(msg.Value))

        // Mark message as processed
        session.MarkMessage(msg, "")
    }
    return nil
}

// Create consumer group
consumer, err := kafkaagent.NewConsumerGroup(kafkaagent.ConsumerConfig{
    Brokers: []string{"localhost:9092"},
    GroupID: "my-consumer-group",
})
defer consumer.Close()

// Wrap handler for automatic tracing
handler := kafkaagent.WrapConsumerGroupHandler(&MyHandler{})

// Consume messages (automatically traced)
consumer.Consume(ctx, []string{"my-topic"}, handler)
```

## 🌐 HTTP Client Support

```go
import (
    "net/http"
    "net/http/httptrace"
    httpagent "github.com/last9/go-agent/integrations/http"
    "go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
)

// Create instrumented client
client := httpagent.NewClient(&http.Client{
    Timeout: 10 * time.Second,
})

// Make request with proper trace nesting
ctx = httptrace.WithClientTrace(ctx, otelhttptrace.NewClientTrace(ctx))
req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/data", nil)
resp, err := client.Do(req)
```

## 📊 Metrics Support

The agent automatically collects metrics for all integrated services and provides helpers for custom metrics.

### Automatic Metrics

All integrations collect metrics automatically - no additional code needed:

#### Runtime Metrics (Automatic)
- `process.runtime.go.mem.heap_alloc` - Heap memory usage
- `process.runtime.go.goroutines` - Number of goroutines
- `process.runtime.go.gc.count` - GC cycle count
- `process.runtime.go.gc.pause_ns` - GC pause duration

#### HTTP/gRPC Metrics (Automatic)
- `http.server.request.duration` - Server request latency
- `http.server.request.body.size` - Request size
- `http.server.response.body.size` - Response size
- `http.server.active_requests` - Active requests (gauge)
- `rpc.server.duration` - gRPC server request latency
- `rpc.server.request.size` - gRPC request size
- `rpc.server.response.size` - gRPC response size

#### Database Metrics (Automatic)
- `db.client.connections.usage` - Active connections
- `db.client.connections.idle` - Idle connections
- `db.client.connections.max` - Max connections
- `db.client.connections.wait_time` - Time to acquire connection
- `db.client.connections.use_time` - Connection usage duration
- `db.client.connections.idle_time` - Connection idle duration

#### Kafka Metrics (Automatic)
- `messaging.kafka.messages.sent` - Messages produced
- `messaging.kafka.messages.received` - Messages consumed
- `messaging.kafka.messages.errors` - Producer errors
- `messaging.kafka.receive.errors` - Consumer errors
- `messaging.kafka.send.duration` - Producer latency
- `messaging.kafka.process.duration` - Consumer processing time
- `messaging.kafka.message.size` - Message size distribution

#### Redis Metrics (Automatic)
Uses [OpenTelemetry Database Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/database/database-metrics/):
- `db.client.connections.usage` - Number of connections in use
- `db.client.connections.max` - Maximum number of connections
- `db.client.connections.idle.max` - Maximum idle connections
- `db.client.connections.idle.min` - Minimum idle connections
- `db.client.connections.waits` - Connection wait count
- `db.client.connections.waits_duration` - Time waiting for connection
- `db.client.connections.timeouts` - Connection timeout count
- `db.client.connections.create_time` - Connection creation time
- `db.client.connections.use_time` - Connection usage time

### Custom Application Metrics

Add your own business metrics using the `metrics` package:

#### Counter - Monotonically increasing values

```go
import "github.com/last9/go-agent/metrics"

// Create counter
requestCounter := metrics.NewCounter(
    "app.requests.total",
    "Total number of requests processed",
    "{request}",
)

// Increment
requestCounter.Inc(ctx,
    attribute.String("endpoint", "/api/users"),
    attribute.String("method", "GET"),
)

// Add specific value
requestCounter.Add(ctx, 5, attribute.String("batch", "yes"))
```

#### Histogram - Distribution of values

```go
// Create histogram for latency
latencyHistogram := metrics.NewHistogram(
    "app.processing.duration",
    "Processing duration in milliseconds",
    "ms",
)

// Record value
start := time.Now()
// ... do work ...
duration := time.Since(start).Milliseconds()
latencyHistogram.Record(ctx, duration,
    attribute.String("operation", "compute"),
)
```

#### Gauge - Current value (async callback)

```go
var activeWorkers int64

// Create gauge with callback
workerGauge := metrics.NewGauge(
    "app.workers.active",
    "Number of active worker goroutines",
    "{worker}",
    func(ctx context.Context) int64 {
        return atomic.LoadInt64(&activeWorkers)
    },
)

// Gauge is automatically updated by callback
atomic.AddInt64(&activeWorkers, 1)  // Increment
// ... gauge reflects new value on next collection
```

#### UpDownCounter - Value that can increase or decrease

```go
// Create up-down counter for queue
queueSize := metrics.NewUpDownCounter(
    "app.queue.size",
    "Number of items in processing queue",
    "{item}",
)

// Add items
queueSize.Add(ctx, 10, attribute.String("queue", "high-priority"))

// Remove items (negative value)
queueSize.Add(ctx, -5, attribute.String("queue", "high-priority"))
```

### Metric Units

Use standard UCUM units for consistency:
- **Time**: `ms` (milliseconds), `s` (seconds)
- **Bytes**: `By` (bytes), `kBy` (kilobytes), `MBy` (megabytes)
- **Count**: `{item}`, `{request}`, `{error}`, `{connection}`
- **Percent**: `%`

### Complete Metrics Example

```go
package main

import (
    "context"
    "time"

    "github.com/last9/go-agent"
    "github.com/last9/go-agent/metrics"
    "go.opentelemetry.io/otel/attribute"
)

var (
    // Business metrics
    ordersProcessed = metrics.NewCounter(
        "app.orders.processed",
        "Total orders processed",
        "{order}",
    )

    orderValue = metrics.NewFloatHistogram(
        "app.order.value",
        "Order value in USD",
        "USD",
    )

    processingDuration = metrics.NewHistogram(
        "app.order.processing.duration",
        "Order processing duration",
        "ms",
    )
)

func main() {
    agent.Start()
    defer agent.Shutdown()

    ctx := context.Background()

    // Process order
    start := time.Now()
    processOrder(ctx, 123.45)
    duration := time.Since(start).Milliseconds()

    // Record metrics
    ordersProcessed.Inc(ctx, attribute.String("status", "success"))
    orderValue.Record(ctx, 123.45, attribute.String("category", "electronics"))
    processingDuration.Record(ctx, duration)
}

func processOrder(ctx context.Context, value float64) {
    // Business logic...
}
```

## ⚙️ Configuration

The agent reads configuration from environment variables following OpenTelemetry standards:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Yes | - | Last9 OTLP endpoint |
| `OTEL_EXPORTER_OTLP_HEADERS` | Yes | - | Authorization header |
| `OTEL_SERVICE_NAME` | No | `unknown-service` | Service name |
| `OTEL_SERVICE_VERSION` | No | - | Service version (e.g., git commit SHA) |
| `OTEL_RESOURCE_ATTRIBUTES` | No | - | Additional attributes (key=value pairs) |
| `OTEL_TRACES_SAMPLER` | No | `always_on` | Sampling strategy |

### Resource Attributes

The agent automatically captures:
- **Service info**: `service.name`, `service.version`
- **Environment**: `deployment.environment` (defaults to "production")
- **Host info**: hostname, OS, architecture
- **Container**: container ID (if running in container)
- **Process**: process ID, command line
- **Custom attributes**: Any additional key-value pairs from `OTEL_RESOURCE_ATTRIBUTES`


## 📊 What Gets Traced?

### Automatic Instrumentation:
- ✅ HTTP requests (endpoint, method, status code, duration)
- ✅ gRPC calls (service, method, status code)
- ✅ Database queries (query, duration, rows affected)
- ✅ Redis commands (command, duration)
- ✅ Kafka messages (topic, partition, offset, context propagation)
- ✅ External API calls (URL, method, status code)
- ✅ Errors and exceptions
- ✅ Distributed trace context propagation

### Automatic Metrics:
- ✅ **Runtime**: Go memory (heap alloc), goroutines, GC cycles/pause times, CPU time
- ✅ **HTTP/gRPC**: Request duration, request/response sizes, active requests, RPC latency
- ✅ **Database**: Connection pool (usage, idle, max, wait/use/idle times)
- ✅ **Kafka**: Messages sent/received, errors, send/process duration, message sizes
- ✅ **Redis**: Pool usage, command duration, operation counts

For detailed metrics list and custom metrics, see the [Metrics Support](#-metrics-support) section.

## 🧪 Testing

### Running Tests Locally

The go-agent has comprehensive integration tests that verify instrumentation with real services.

#### Prerequisites

- Docker and Docker Compose (for integration tests)
- Go 1.22+ installed
- `buf` CLI (for generating proto files): `go install github.com/bufbuild/buf/cmd/buf@latest`

#### Quick Start

```bash
# Run all tests (unit + integration)
make test

# Run only unit tests (fast, no Docker required)
make test-unit

# Run only integration tests (requires Docker)
make docker-up          # Start test services
make test-integration   # Run integration tests
make docker-down        # Stop test services
```

#### Manual Setup

```bash
# 1. Start test services (Kafka, PostgreSQL, Redis, MySQL)
docker-compose -f docker-compose.test.yml up -d

# 2. Wait for services to be ready (automatic in Makefile)
# Check with: docker-compose -f docker-compose.test.yml ps

# 3. Run tests
go test -v ./...                                    # Unit tests
go test -v -tags=integration ./tests/integration/  # Integration tests

# 4. Stop services
docker-compose -f docker-compose.test.yml down -v
```

## 🏗️ Build Tags and Go Version Support

The go-agent uses Go build tags to provide optimal functionality across different Go versions. See [Go Version Feature Matrix](#go-version-feature-matrix) for details on what each version supports.

### How It Works
The agent automatically detects your Go version at compile time:

```bash
# Go 1.24+ gets full OTel runtime instrumentation
go build  # Uses agent_runtime_go124.go

# Go 1.22-1.23 gets basic runtime metrics
go build  # Uses agent_runtime_legacy.go
```

### No Configuration Needed
- ✅ Works transparently based on your Go version
- ✅ No environment variables or flags required
- ✅ Compile-time optimization (zero runtime overhead)

## 🤝 Contributing

Contributions welcome! Please:
1. Check existing issues or create a new one
2. Fork the repository
3. Create a feature branch
4. Submit a pull request

## 📄 License

Apache License 2.0 - see LICENSE file for details

## 🆘 Support

- Documentation: https://last9.io/docs
- Issues: https://github.com/last9/go-agent/issues

## 🙏 Acknowledgments

Built on top of:
- [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go)
- [OpenTelemetry Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib)
