# Last9 Go Agent

A drop-in OpenTelemetry agent for Go applications that minimizes code changes while providing comprehensive observability with automatic traces and metrics.

[![CI](https://github.com/last9/go-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/last9/go-agent/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/last9/go-agent)](https://goreportcard.com/report/github.com/last9/go-agent)
[![codecov](https://codecov.io/gh/last9/go-agent/branch/main/graph/badge.svg)](https://codecov.io/gh/last9/go-agent)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Reference](https://pkg.go.dev/badge/github.com/last9/go-agent.svg)](https://pkg.go.dev/github.com/last9/go-agent)

**87% less boilerplate** • **2-line setup** • **Automatic traces + metrics** • **Production-ready**

## 📋 Table of Contents

- [Key Features](#-key-features)
- [Quick Start](#-sdk-quick-start-this-repo)
- [Framework Support](#-framework-support) - Gin • Chi • Echo • Gorilla • gRPC-Gateway
- [Database Support](#️-database-support) - PostgreSQL • MySQL • SQLite
- [Redis Support](#-redis-support)
- [Kafka Support](#-kafka-support) - Producers • Consumers
- [HTTP Client](#-http-client-support)
- [Metrics Support](#-metrics-support) - Automatic • Custom • Runtime
- [Configuration](#️-configuration)
- [Examples](#-complete-example)
- [Testing](#-testing) - Running Tests • CI/CD
- [SDK vs eBPF](#-sdk-vs-ebpf-full-comparison)

## ✨ Key Features

- 🚀 **One-line initialization** - `agent.Start()` replaces 150+ lines of OpenTelemetry setup
- 🔌 **Drop-in replacements** - Minimal code changes for Gin, Chi, Echo, Gorilla, gRPC-Gateway
- 🎯 **Auto-instrumentation** - HTTP, gRPC, SQL, Redis, Kafka automatically traced with proper span nesting
- 📊 **Automatic metrics** - Runtime (memory, GC, goroutines), HTTP, gRPC, database, Kafka, Redis metrics out-of-the-box
- 📈 **Custom metrics** - Simple helpers for counters, histograms, gauges for business metrics
- ⚙️ **Zero-config** - Reads from standard OpenTelemetry environment variables
- 🔍 **Complete observability** - Full distributed tracing + metrics across all layers (HTTP → gRPC → DB → External APIs)
- 🏭 **Production-tested** - Battle-tested across 70+ microservices

## 🎯 Problem

Existing OpenTelemetry integrations require significant code changes:
- 150+ lines of boilerplate for initialization
- Manual instrumentation for each framework, database, and service
- Complex configuration management
- Difficult to scale across 70+ microservices

## ✨ Solution

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
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp.last9.io"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic YOUR_TOKEN"
export OTEL_SERVICE_NAME="my-service"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,team=backend"
```

### 3. Instrument

#### Before (150+ lines of boilerplate):

```go
func main() {
    // 150+ lines of OpenTelemetry setup...
    exporter, _ := otlptracehttp.New(context.Background())
    tp := sdktrace.NewTracerProvider(...)
    otel.SetTracerProvider(tp)
    // ... more boilerplate

    r := gin.Default()
    r.Use(otelgin.Middleware("my-service"))
    // ... routes
}
```

#### After (2 lines):

```go
import (
    "github.com/last9/go-agent"
    ginagent "github.com/last9/go-agent/instrumentation/gin"
)

func main() {
    agent.Start()
    defer agent.Shutdown()

    r := ginagent.Default() // Drop-in replacement!
    // ... routes
}
```

## 📚 Framework Support

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

## 📖 Complete Example

```go
package main

import (
    "context"
    "log"

    "github.com/last9/go-agent"
    ginagent "github.com/last9/go-agent/instrumentation/gin"
    "github.com/last9/go-agent/integrations/database"
    redisagent "github.com/last9/go-agent/integrations/redis"
)

func main() {
    // 1. Start agent (reads from environment variables)
    if err := agent.Start(); err != nil {
        log.Fatal(err)
    }
    defer agent.Shutdown()

    // 2. Setup database (automatically instrumented)
    db := database.MustOpen(database.Config{
        DriverName:   "postgres",
        DSN:          "postgres://localhost/mydb",
        DatabaseName: "mydb",
    })
    defer db.Close()

    // 3. Setup Redis (automatically instrumented)
    rdb := redisagent.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    defer rdb.Close()

    // 4. Setup Gin router (automatically instrumented)
    r := ginagent.Default()

    r.GET("/users/:id", func(c *gin.Context) {
        id := c.Param("id")

        // Database query (automatically traced)
        var name string
        err := db.QueryRow("SELECT name FROM users WHERE id = $1", id).Scan(&name)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

        // Redis cache (automatically traced)
        cacheKey := "user:" + id
        rdb.Set(c.Request.Context(), cacheKey, name, 0)

        c.JSON(200, gin.H{"id": id, "name": name})
    })

    r.Run(":8080")
}
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
- `redis.pool.connections.usage` - Pool usage
- `redis.pool.connections.idle` - Idle connections
- `redis.command.duration` - Command latency

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

### Example Configuration:

```bash
# Last9 Configuration
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp.last9.io"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic YOUR_TOKEN_HERE"

# Service Configuration
export OTEL_SERVICE_NAME="user-service"
export OTEL_SERVICE_VERSION="v1.2.3"  # or use git SHA: $(git rev-parse --short HEAD)

# Resource Attributes
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,team=backend,region=us-east-1,component=api"

# Sampling
export OTEL_TRACES_SAMPLER="always_on"  # or "traceidratio" with OTEL_TRACES_SAMPLER_ARG
```

## 🎯 SDK vs eBPF: Full Comparison

### Feature Comparison

| Feature | Last9 SDK (This Repo) | Last9 eBPF (Operator) | Raw OpenTelemetry |
|---------|----------------------|----------------------|-------------------|
| **Setup** | 2 lines | 0 lines (annotation) | 150+ lines |
| **Environment** | Anywhere | Kubernetes only | Anywhere |
| **Privileges** | None | eBPF/root | None |
| **Custom spans** | ✅ Yes | ❌ No | ✅ Yes |
| **3rd-party libs** | Via wrappers | ✅ Automatic | Via wrappers |
| **Local dev** | ✅ Yes | ❌ No | ✅ Yes |
| **Lambda/VMs** | ✅ Yes | ❌ No | ✅ Yes |
| **Learning curve** | Minimal | Very low | High |

### When to Combine Both

Use **both** for maximum coverage:

```go
// SDK provides custom spans for business logic
import "github.com/last9/go-agent"

func ProcessPayment(ctx context.Context) {
    // eBPF automatically traces HTTP and DB calls
    // SDK adds custom business logic spans
    span := agent.StartSpan(ctx, "process-payment")
    defer span.End()

    // Your business logic
    validateCard()  // eBPF traces DB call automatically
    chargeAmount()  // SDK provides custom span
}
```

**Deploy:** Add eBPF annotation in K8s, SDK custom spans work automatically!

## 🏗️ Architecture

```
last9/go-agent/
├── agent.go                    # Core agent (Start/Shutdown)
├── config/                     # Configuration management
├── instrumentation/
│   ├── gin/                    # Gin framework
│   ├── chi/                    # Chi framework
│   ├── echo/                   # Echo framework
│   ├── gorilla/                # Gorilla Mux
│   └── grpcgateway/            # gRPC-Gateway
├── integrations/
│   ├── database/               # SQL instrumentation
│   ├── redis/                  # Redis instrumentation
│   ├── kafka/                  # Kafka instrumentation (IBM Sarama)
│   └── http/                   # HTTP client instrumentation
└── tests/                      # Integration tests
```

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

### Test Structure

```
tests/
├── testutil/              # Test utilities
│   ├── otel_collector.go  # Mock OTLP collector
│   ├── span_assertions.go # Span verification helpers
│   └── context_helpers.go # Context propagation utilities
└── integration/           # Integration tests
    ├── kafka_test.go      # Kafka producer/consumer tests
    ├── grpc_gateway_test.go  # gRPC-Gateway full stack tests
    └── testdata/          # Test protobuf definitions
```

### Integration Tests

Integration tests use **testcontainers-go** to run real services:
- **Kafka**: Producer/consumer with end-to-end context propagation
- **gRPC-Gateway**: Full HTTP → gRPC → Handler flow
- **PostgreSQL/MySQL**: Database query instrumentation
- **Redis**: Command instrumentation

Tests verify:
- ✅ Spans are created correctly
- ✅ Trace context propagates across service boundaries
- ✅ Semantic conventions are followed
- ✅ Error handling works properly

### Linting

```bash
# Install golangci-lint
# See: https://golangci-lint.run/usage/install/

# Run linter
make lint
```

### CI/CD

The project uses GitHub Actions for continuous integration:
- **Lint**: Code quality checks with golangci-lint
- **Unit Tests**: Fast tests across Go 1.22, 1.23, 1.24
- **Integration Tests**: Real service tests with Kafka, Postgres, Redis, MySQL
- **Build**: Verify all packages and examples compile

See [`.github/workflows/ci.yml`](.github/workflows/ci.yml) for details.

### Writing Tests

When adding new instrumentation, follow this pattern:

```go
//go:build integration

package integration

func TestMyIntegration_Tracing(t *testing.T) {
    // 1. Setup mock collector
    collector := testutil.NewMockCollector()
    defer collector.Shutdown(context.Background())

    // 2. Initialize agent
    agent.Start()
    defer agent.Shutdown()

    // 3. Perform instrumented operation
    // ... your test code ...

    // 4. Verify spans
    spans := collector.GetSpans()
    testutil.AssertSpanCount(t, spans, expectedCount)
    testutil.AssertSpanAttribute(t, spans[0], "key", "value")
}
```

See [`tests/integration/kafka_test.go`](tests/integration/kafka_test.go) for a complete example.

## 🔮 Roadmap

- [x] Gin framework support
- [x] Chi framework support
- [x] Echo framework support
- [x] Gorilla Mux support
- [x] gRPC-Gateway support
- [x] PostgreSQL / MySQL / SQLite support
- [x] Redis support
- [x] Kafka support (IBM Sarama)
- [x] HTTP client instrumentation
- [x] Resource attributes configuration
- [x] Automatic runtime metrics
- [x] Custom metrics helpers
- [ ] Fiber framework support
- [ ] MongoDB instrumentation
- [ ] Custom span creation helpers

## 🤝 Contributing

Contributions welcome! Please:
1. Check existing issues or create a new one
2. Fork the repository
3. Create a feature branch
4. Submit a pull request

## 📄 License

Apache License 2.0 - see LICENSE file for details

## 🆘 Support

- Documentation: https://docs.last9.io
- Issues: https://github.com/last9/go-agent/issues
- Community: https://discord.gg/last9

## 🙏 Acknowledgments

Built on top of:
- [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go)
- [OpenTelemetry Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib)
