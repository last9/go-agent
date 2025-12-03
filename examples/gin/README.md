# Gin Framework Example

This example demonstrates how to use the Last9 Go Agent with the Gin web framework.

## Features Demonstrated

- ✅ Automatic HTTP request tracing
- ✅ Route parameter tracing
- ✅ Error handling and tracing
- ✅ External API calls
- ✅ Graceful shutdown
- ✅ Health check endpoints

## Running the Example

### 1. Configure Environment

Copy the example environment file and add your Last9 credentials:

```bash
cp .env.example .env
```

Edit `.env` with your Last9 OTLP endpoint and authorization token:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.last9.io
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic YOUR_TOKEN_HERE
OTEL_SERVICE_NAME=gin-example
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=development,team=platform
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Run the Application

```bash
source .env  # Load environment variables
go run main.go
```

The server will start on `http://localhost:8080`

### 4. Test Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Simple ping
curl http://localhost:8080/ping

# Route with parameters
curl http://localhost:8080/hello/World

# POST request with JSON body
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name":"John Doe","email":"john@example.com"}'

# External API call (traces the outgoing request)
curl http://localhost:8080/external

# Error endpoint (demonstrates error tracing)
curl http://localhost:8080/error
```

## Code Highlights

### Minimal Setup

```go
// Just 2 lines to enable full instrumentation!
agent.Start()
defer agent.Shutdown()

r := ginagent.Default()
```

### Drop-in Replacement

```go
// Instead of:
// r := gin.Default()

// Use:
r := ginagent.Default()
```

That's it! All routes are automatically instrumented.

## What Gets Traced?

When you make requests to the endpoints, you'll see in Last9:

1. **HTTP Requests**:
   - Endpoint path (e.g., `/hello/:name`)
   - HTTP method (GET, POST, etc.)
   - Status code
   - Duration
   - Request headers

2. **Errors**:
   - Error messages
   - Stack traces
   - Error context

3. **External Calls**:
   - Outgoing HTTP requests
   - Response times
   - Status codes

4. **Metrics**:
   - Request rate
   - Error rate
   - Latency percentiles (p50, p95, p99)
   - Active requests

## Next Steps

Check out more advanced examples:
- `examples/gin-with-database/` - Database instrumentation
- `examples/gin-with-redis/` - Redis instrumentation
- `examples/gin-full/` - Complete application with DB, Redis, and more
