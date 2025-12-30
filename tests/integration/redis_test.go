//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	redisagent "github.com/last9/go-agent/integrations/redis"
	"github.com/last9/go-agent/tests/testutil"
)

func setupRedisTest(t *testing.T) (*tcredis.RedisContainer, string, context.Context, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Start Redis container
	redisContainer, err := tcredis.RunContainer(ctx,
		testcontainers.WithImage("redis:7-alpine"),
	)
	require.NoError(t, err, "failed to start Redis container")

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := redisContainer.Terminate(cleanupCtx); err != nil {
			t.Logf("Warning: failed to terminate Redis container: %v", err)
		}
		cancel()
	})

	// Get connection string
	endpoint, err := redisContainer.Endpoint(ctx, "")
	require.NoError(t, err)

	return redisContainer, endpoint, ctx, cancel
}

func TestRedis_SetGet(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	// Create instrumented Redis client
	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	// Verify connection
	err = rdb.Ping(ctx).Err()
	require.NoError(t, err)

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-redis-parent")
	defer parentSpan.End()

	// Set a value
	err = rdb.Set(ctx, "test-key", "test-value", time.Minute).Err()
	require.NoError(t, err)

	// Get the value
	val, err := rdb.Get(ctx, "test-key").Result()
	require.NoError(t, err)
	assert.Equal(t, "test-value", val)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	// Should have parent + SET + GET spans (at least)
	require.GreaterOrEqual(t, len(spans), 3, "should have at least 3 spans")

	// Find client spans
	clientSpans := testutil.FindSpansByKind(spans, trace.SpanKindClient)
	require.GreaterOrEqual(t, len(clientSpans), 2, "should have at least 2 Redis client spans")
}

func TestRedis_Hash(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-hash-parent")
	defer parentSpan.End()

	// Set hash fields
	err = rdb.HSet(ctx, "user:1", map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	}).Err()
	require.NoError(t, err)

	// Get hash field
	name, err := rdb.HGet(ctx, "user:1", "name").Result()
	require.NoError(t, err)
	assert.Equal(t, "Alice", name)

	// Get all hash fields
	all, err := rdb.HGetAll(ctx, "user:1").Result()
	require.NoError(t, err)
	assert.Equal(t, "Alice", all["name"])
	assert.Equal(t, "alice@example.com", all["email"])

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 4, "should have hash operation spans")
}

func TestRedis_List(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-list-parent")
	defer parentSpan.End()

	// Push to list
	err = rdb.RPush(ctx, "mylist", "item1", "item2", "item3").Err()
	require.NoError(t, err)

	// Get list length
	length, err := rdb.LLen(ctx, "mylist").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(3), length)

	// Pop from list
	val, err := rdb.LPop(ctx, "mylist").Result()
	require.NoError(t, err)
	assert.Equal(t, "item1", val)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 4, "should have list operation spans")
}

func TestRedis_Pipeline(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-pipeline-parent")
	defer parentSpan.End()

	// Use pipeline
	pipe := rdb.Pipeline()
	pipe.Set(ctx, "key1", "value1", time.Minute)
	pipe.Set(ctx, "key2", "value2", time.Minute)
	pipe.Get(ctx, "key1")

	_, err = pipe.Exec(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "should have pipeline spans")
}

func TestRedis_TTL(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-ttl-parent")
	defer parentSpan.End()

	// Set with TTL
	err = rdb.Set(ctx, "expiring-key", "value", 10*time.Second).Err()
	require.NoError(t, err)

	// Check TTL
	ttl, err := rdb.TTL(ctx, "expiring-key").Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 10*time.Second)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 3, "should have SET and TTL spans")
}

func TestRedis_Delete(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-delete-parent")
	defer parentSpan.End()

	// Set keys
	err = rdb.Set(ctx, "del-key1", "value1", 0).Err()
	require.NoError(t, err)
	err = rdb.Set(ctx, "del-key2", "value2", 0).Err()
	require.NoError(t, err)

	// Delete keys
	deleted, err := rdb.Del(ctx, "del-key1", "del-key2").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify deletion
	exists, err := rdb.Exists(ctx, "del-key1").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 5, "should have SET, DEL, EXISTS spans")
}

func TestRedis_KeyNotFound(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-notfound-parent")
	defer parentSpan.End()

	// Try to get non-existent key
	_, err = rdb.Get(ctx, "nonexistent-key").Result()
	assert.ErrorIs(t, err, redis.Nil)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "should have GET span even for Nil result")
}

func TestRedis_ContextPropagation(t *testing.T) {
	_, endpoint, ctx, cancel := setupRedisTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	rdb, err := redisagent.NewClient(&redis.Options{
		Addr: endpoint,
	})
	require.NoError(t, err)
	defer rdb.Close()

	// Create parent span
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "redis-context-test")
	parentTraceID := testutil.GetTraceIDFromContext(ctx)
	defer parentSpan.End()

	// Execute Redis command
	err = rdb.Set(ctx, "ctx-key", "ctx-value", time.Minute).Err()
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)

	// Verify all spans are in the same trace
	testutil.AssertSpanInSameTrace(t, spans)

	// Find Redis span and verify it's in the same trace as parent
	redisSpan := testutil.FindSpanByKind(spans, trace.SpanKindClient)
	require.NotNil(t, redisSpan)
	assert.Equal(t, parentTraceID, redisSpan.SpanContext().TraceID())
}
