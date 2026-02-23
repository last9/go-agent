//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	mongoagent "github.com/last9/go-agent/integrations/mongodb"
	"github.com/last9/go-agent/tests/testutil"
)

func setupMongoDBTest(t *testing.T) (*tcmongodb.MongoDBContainer, string, context.Context, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	mongoContainer, err := tcmongodb.RunContainer(ctx,
		testcontainers.WithImage("mongo:7"),
	)
	require.NoError(t, err, "failed to start MongoDB container")

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := mongoContainer.Terminate(cleanupCtx); err != nil {
			t.Logf("Warning: failed to terminate MongoDB container: %v", err)
		}
		cancel()
	})

	uri, err := mongoContainer.ConnectionString(ctx)
	require.NoError(t, err)

	return mongoContainer, uri, ctx, cancel
}

func TestMongoDB_InsertFind(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	// Create collector BEFORE creating the client so the client's tracer
	// uses the mock collector's tracer provider.
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	client, err := mongoagent.NewClient(mongoagent.Config{
		URI: uri,
	})
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	// Verify connection
	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-mongodb-parent")
	defer parentSpan.End()

	db := client.Database("testdb")
	col := db.Collection("users")

	// Insert a document
	_, err = col.InsertOne(ctx, bson.M{"name": "Alice", "age": 30})
	require.NoError(t, err)

	// Find the document
	var result bson.M
	err = col.FindOne(ctx, bson.M{"name": "Alice"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Alice", result["name"])

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	clientSpans := testutil.FindSpansByKind(spans, trace.SpanKindClient)
	require.GreaterOrEqual(t, len(clientSpans), 2, "should have at least 2 MongoDB client spans")

	// Verify insert span
	insertSpan := testutil.FindSpanByName(spans, "insert users")
	require.NotNil(t, insertSpan, "should have an insert span")
	testutil.AssertSpanAttribute(t, insertSpan, "db.system", "mongodb")
	testutil.AssertSpanAttribute(t, insertSpan, "db.operation", "insert")
	testutil.AssertSpanAttribute(t, insertSpan, "db.mongodb.collection", "users")
	testutil.AssertSpanAttribute(t, insertSpan, "db.name", "testdb")

	// Verify find span
	findSpan := testutil.FindSpanByName(spans, "find users")
	require.NotNil(t, findSpan, "should have a find span")
	testutil.AssertSpanAttribute(t, findSpan, "db.system", "mongodb")
	testutil.AssertSpanAttribute(t, findSpan, "db.operation", "find")
	testutil.AssertSpanAttribute(t, findSpan, "db.name", "testdb")
	testutil.AssertSpanAttribute(t, findSpan, "db.mongodb.collection", "users")
}

func TestMongoDB_UpdateDelete(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	client, err := mongoagent.NewClient(mongoagent.Config{
		URI: uri,
	})
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-update-delete-parent")
	defer parentSpan.End()

	col := client.Database("testdb").Collection("items")

	// Insert
	_, err = col.InsertOne(ctx, bson.M{"name": "widget", "count": 10})
	require.NoError(t, err)

	// Update
	_, err = col.UpdateOne(ctx, bson.M{"name": "widget"}, bson.M{"$set": bson.M{"count": 20}})
	require.NoError(t, err)

	// Delete
	_, err = col.DeleteOne(ctx, bson.M{"name": "widget"})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	updateSpan := testutil.FindSpanByName(spans, "update items")
	require.NotNil(t, updateSpan, "should have an update span")
	testutil.AssertSpanAttribute(t, updateSpan, "db.system", "mongodb")
	testutil.AssertSpanAttribute(t, updateSpan, "db.operation", "update")
	testutil.AssertSpanAttribute(t, updateSpan, "db.mongodb.collection", "items")
	testutil.AssertSpanAttribute(t, updateSpan, "db.name", "testdb")

	deleteSpan := testutil.FindSpanByName(spans, "delete items")
	require.NotNil(t, deleteSpan, "should have a delete span")
	testutil.AssertSpanAttribute(t, deleteSpan, "db.system", "mongodb")
	testutil.AssertSpanAttribute(t, deleteSpan, "db.operation", "delete")
	testutil.AssertSpanAttribute(t, deleteSpan, "db.mongodb.collection", "items")
}

func TestMongoDB_ContextPropagation(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	client, err := mongoagent.NewClient(mongoagent.Config{
		URI: uri,
	})
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	// Create parent span
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "mongodb-context-test")
	parentTraceID := testutil.GetTraceIDFromContext(ctx)

	col := client.Database("testdb").Collection("users")
	_, err = col.InsertOne(ctx, bson.M{"name": "Bob"})
	require.NoError(t, err)

	// End parent span so it appears in the collector
	parentSpan.End()

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)

	// Find the insert span specifically
	insertSpan := testutil.FindSpanByName(spans, "insert users")
	require.NotNil(t, insertSpan, "should have an insert span")

	// Verify it's in the same trace as parent
	assert.Equal(t, parentTraceID, insertSpan.SpanContext().TraceID())

	// Verify all spans share the same trace
	testutil.AssertSpanInSameTrace(t, spans)
}

func TestMongoDB_Aggregate(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	client, err := mongoagent.NewClient(mongoagent.Config{
		URI: uri,
	})
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-aggregate-parent")
	defer parentSpan.End()

	col := client.Database("testdb").Collection("orders")

	// Insert some documents
	docs := []interface{}{
		bson.M{"item": "apple", "quantity": 5, "price": 1.0},
		bson.M{"item": "banana", "quantity": 10, "price": 0.5},
		bson.M{"item": "apple", "quantity": 3, "price": 1.0},
	}
	_, err = col.InsertMany(ctx, docs)
	require.NoError(t, err)

	// Run an aggregation pipeline
	pipeline := bson.A{
		bson.M{"$group": bson.M{
			"_id":   "$item",
			"total": bson.M{"$sum": "$quantity"},
		}},
	}
	cursor, err := col.Aggregate(ctx, pipeline)
	require.NoError(t, err)

	var results []bson.M
	err = cursor.All(ctx, &results)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()

	aggregateSpan := testutil.FindSpanByName(spans, "aggregate orders")
	require.NotNil(t, aggregateSpan, "should have an aggregate span")
	testutil.AssertSpanAttribute(t, aggregateSpan, "db.system", "mongodb")
	testutil.AssertSpanAttribute(t, aggregateSpan, "db.operation", "aggregate")
	testutil.AssertSpanAttribute(t, aggregateSpan, "db.mongodb.collection", "orders")
	testutil.AssertSpanAttribute(t, aggregateSpan, "db.name", "testdb")
}

func TestMongoDB_SkippedCommands(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	client, err := mongoagent.NewClient(mongoagent.Config{
		URI: uri,
	})
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	// Ping is in the skip list — should NOT produce a span
	// hello/isMaster from connection setup are also skipped.
	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	// All spans should be from the initial connection (hello, isMaster, ping)
	// which are all in the default skip list — no spans expected
	for _, span := range spans {
		assert.NotEqual(t, "ping", span.Name(), "ping should be skipped")
		assert.NotContains(t, span.Name(), "hello", "hello should be skipped")
		assert.NotContains(t, span.Name(), "isMaster", "isMaster should be skipped")
	}
}

func TestMongoDB_DuplicateKeyStillTraced(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	client, err := mongoagent.NewClient(mongoagent.Config{
		URI: uri,
	})
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-error-parent")
	defer parentSpan.End()

	col := client.Database("testdb").Collection("unique_items")

	// Create a unique index
	_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.M{"name": 1},
		Options: options.Index().SetUnique(true),
	})
	require.NoError(t, err)

	// Insert first document
	_, err = col.InsertOne(ctx, bson.M{"name": "unique_widget"})
	require.NoError(t, err)

	// Insert duplicate — returns an application-level error, but the MongoDB
	// command itself "succeeds" at the wire protocol level. The CommandMonitor
	// fires Succeeded (not Failed) for this case. We verify the span is still
	// created for the failed insert.
	_, err = col.InsertOne(ctx, bson.M{"name": "unique_widget"})
	require.Error(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	// Both insert operations should produce spans (even the one that returned
	// an application error), plus createIndexes
	insertCount := 0
	for _, span := range spans {
		if span.Name() == "insert unique_items" {
			insertCount++
		}
	}
	assert.Equal(t, 2, insertCount, "both insert operations should produce spans")
}

func TestMongoDB_InstrumentAPI(t *testing.T) {
	_, uri, ctx, cancel := setupMongoDBTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	// Use Instrument() with pre-built options
	opts := options.Client().ApplyURI(uri)
	client, err := mongoagent.Instrument(opts)
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err)

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-instrument-api")
	defer parentSpan.End()

	col := client.Database("testdb").Collection("items")
	_, err = col.InsertOne(ctx, bson.M{"name": "gadget"})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()

	insertSpan := testutil.FindSpanByName(spans, "insert items")
	require.NotNil(t, insertSpan, "Instrument() should produce traced spans")
	testutil.AssertSpanAttribute(t, insertSpan, "db.system", "mongodb")
	testutil.AssertSpanAttribute(t, insertSpan, "db.operation", "insert")
}
