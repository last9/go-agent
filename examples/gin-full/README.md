# Full-Featured Gin Example

This example demonstrates a complete application with all Last9 Go Agent features:

- ✅ HTTP request tracing (Gin)
- ✅ Database instrumentation (PostgreSQL)
- ✅ Redis instrumentation
- ✅ External API tracing (HTTP client)
- ✅ Health checks
- ✅ Error handling

## Prerequisites

- Go 1.21+
- PostgreSQL (optional)
- Redis (optional)

## Quick Start (No Database Required)

You can run this example without PostgreSQL - it will work with Redis and HTTP tracing:

```bash
# 1. Start Redis (using Docker)
docker run -d -p 6379:6379 redis:7-alpine

# 2. Configure environment
cp .env.example .env
# Edit .env with your Last9 credentials

# 3. Run the application
source .env
go run main.go
```

## Full Setup (With Database)

### 1. Start Dependencies

```bash
# Start PostgreSQL and Redis using Docker Compose
docker-compose up -d

# Or start them separately:
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15-alpine
docker run -d -p 6379:6379 redis:7-alpine
```

### 2. Setup Database

```bash
# Create database
createdb example

# Create users table
psql example << EOF
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Insert sample data
INSERT INTO users (name, email) VALUES
    ('Alice', 'alice@example.com'),
    ('Bob', 'bob@example.com'),
    ('Charlie', 'charlie@example.com');
EOF
```

### 3. Configure Environment

```bash
cp .env.example .env
```

Edit `.env`:

```bash
# Last9 Configuration
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.last9.io
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic YOUR_TOKEN
OTEL_SERVICE_NAME=gin-full-example

# Database
DATABASE_URL=postgres://postgres:postgres@localhost/example?sslmode=disable

# Redis
REDIS_URL=localhost:6379
```

### 4. Run the Application

```bash
source .env
go run main.go
```

## Testing Endpoints

### 1. Health Check

```bash
curl http://localhost:8080/health
```

Expected output:
```json
{
  "status": "healthy",
  "database": "healthy",
  "redis": "healthy",
  "timestamp": 1234567890
}
```

### 2. List Users (Database Query)

```bash
curl http://localhost:8080/users
```

This demonstrates:
- Automatic database query tracing
- Query duration metrics
- Rows returned

### 3. Get User (Cache + Database)

```bash
# First call: reads from database, writes to cache
curl http://localhost:8080/users/1

# Second call: reads from cache (faster!)
curl http://localhost:8080/users/1
```

This demonstrates:
- Redis GET operation tracing
- Database query tracing
- Redis SET operation tracing
- Cache hit/miss patterns

### 4. Create User (Database Insert)

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name":"David","email":"david@example.com"}'
```

This demonstrates:
- Database INSERT operation tracing
- Request body parsing
- Automatic span attributes

### 5. Cache Operations (Redis)

Set a cache value:
```bash
curl -X POST http://localhost:8080/cache/mykey \
  -H "Content-Type: application/json" \
  -d '{"value":"myvalue","ttl":60}'
```

Get a cache value:
```bash
curl http://localhost:8080/cache/mykey
```

This demonstrates:
- Redis SET with TTL
- Redis GET operations
- Error handling for missing keys

### 6. External API Call (HTTP Client)

```bash
curl http://localhost:8080/external
```

This demonstrates:
- Outgoing HTTP request tracing
- Context propagation to external services
- HTTP client instrumentation

## What You'll See in Last9

### Traces

Each request creates a trace with spans for:

1. **HTTP Request Span**:
   - Service: `gin-full-example`
   - Operation: `GET /users/:id`
   - Attributes: method, path, status code
   - Duration: total request time

2. **Database Query Span** (child of HTTP span):
   - Operation: `SELECT name, email FROM users...`
   - Attributes: db.system (postgresql), db.statement
   - Duration: query execution time

3. **Redis Operation Span** (child of HTTP span):
   - Operation: `GET user:1`
   - Attributes: db.system (redis), db.operation
   - Duration: Redis command time

4. **HTTP Client Span** (child of HTTP span):
   - Operation: `GET https://api.github.com/users/github`
   - Attributes: http.url, http.method, http.status_code
   - Duration: external API call time

### Metrics

- **HTTP metrics**: request rate, latency (p50, p95, p99), error rate
- **Database metrics**: connection pool stats, query duration
- **Redis metrics**: operation duration, hit rate

### Example Trace Flow

```
GET /users/1
├─ redis.GET user:1 (cache miss) - 2ms
├─ postgresql.SELECT - 15ms
│  └─ Query: SELECT name, email FROM users WHERE id = $1
└─ redis.SET user:1 - 1ms
   Total: 18ms
```

## Code Highlights

### Minimal Setup

```go
// Just initialize the agent
agent.Start()
defer agent.Shutdown()

// Everything else is automatic!
```

### Database Operations

```go
// Automatically traced - no manual instrumentation needed!
db.QueryRowContext(ctx, "SELECT * FROM users WHERE id = $1", id)
```

### Redis Operations

```go
// Automatically traced!
rdb.Get(ctx, "user:1").Result()
rdb.Set(ctx, "user:1", "data", time.Minute)
```

### External API Calls

```go
// Create instrumented client
client := httpagent.NewClient(&http.Client{Timeout: 10 * time.Second})

// All requests are automatically traced with context propagation!
client.Do(req)
```

## Performance Impact

The Last9 agent has minimal performance impact:

- **Latency overhead**: < 1ms per request
- **Memory overhead**: ~10-20MB
- **CPU overhead**: < 1%

All telemetry is sent asynchronously in batches.

## Troubleshooting

### Database connection failed

```bash
# Check if PostgreSQL is running
psql -U postgres -h localhost -c "SELECT version();"

# Check connection string
echo $DATABASE_URL
```

### Redis connection failed

```bash
# Check if Redis is running
redis-cli ping

# Should return: PONG
```

### No traces in Last9

1. Check environment variables are set correctly
2. Verify OTLP endpoint is reachable
3. Check authorization token is valid
4. Look for errors in application logs

## Next Steps

- Add more routes and see them automatically traced
- Experiment with different sampling strategies
- Add custom attributes to spans
- Create custom metrics

## Learn More

- [Last9 Documentation](https://docs.last9.io)
- [OpenTelemetry Go Docs](https://opentelemetry.io/docs/instrumentation/go/)
