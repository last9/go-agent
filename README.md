# Last9 Go Agent

A drop-in OpenTelemetry agent for Go applications that minimizes code changes while providing comprehensive observability.

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
- **Pre-built integrations** for databases, Redis, and HTTP clients

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

## 🌐 HTTP Client Support

```go
import httpagent "github.com/last9/go-agent/integrations/http"

// Use instrumented client
client := httpagent.DefaultClient
resp, err := client.Get("https://api.example.com/data")

// Or create a custom client
client := httpagent.NewClient(&http.Client{
    Timeout: 10 * time.Second,
})
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
│   └── gin/                    # Framework integrations
├── integrations/
│   ├── database/               # SQL instrumentation
│   ├── redis/                  # Redis instrumentation
│   └── http/                   # HTTP client instrumentation
└── examples/                   # Usage examples
```

## 🧪 Running the Example

```bash
cd examples/gin

# Copy and edit environment variables
cp .env.example .env
# Edit .env with your Last9 credentials

# Run the example
source .env
go run main.go

# Test endpoints
curl http://localhost:8080/ping
curl http://localhost:8080/hello/World
```

## 📊 What Gets Traced?

### Automatic Instrumentation:
- ✅ HTTP requests (endpoint, method, status code, duration)
- ✅ Database queries (query, duration, rows affected)
- ✅ Redis commands (command, duration)
- ✅ External API calls (URL, method, status code)
- ✅ Errors and exceptions
- ✅ Request context propagation

### Automatic Metrics:
- ✅ HTTP request duration and count
- ✅ Database connection pool stats
- ✅ Redis operation duration and count
- ✅ Go runtime metrics (goroutines, memory, GC)

## 🔮 Roadmap

- [x] Gin framework support
- [x] Chi framework support
- [x] Echo framework support
- [x] Gorilla Mux support
- [x] Resource attributes configuration
- [ ] Fiber framework support
- [ ] gRPC support
- [ ] MongoDB instrumentation
- [ ] Kafka instrumentation
- [ ] Custom span creation helpers
- [ ] Automatic runtime metrics
- [ ] Performance profiling integration

## 🤝 Contributing

Contributions welcome! Please:
1. Check existing issues or create a new one
2. Fork the repository
3. Create a feature branch
4. Submit a pull request

## 📄 License

MIT License - see LICENSE file for details

## 🆘 Support

- Documentation: https://docs.last9.io
- Issues: https://github.com/last9/go-agent/issues
- Community: https://discord.gg/last9

## 🙏 Acknowledgments

Built on top of:
- [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go)
- [OpenTelemetry Go Contrib](https://github.com/open-telemetry/opentelemetry-go-contrib)

Inspired by:
- [New Relic Go Agent](https://github.com/newrelic/go-agent)
- [DataDog Go Tracer](https://github.com/DataDog/dd-trace-go)
