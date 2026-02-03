//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	"github.com/last9/go-agent/integrations/database"
	"github.com/last9/go-agent/tests/testutil"
)

func setupPostgresTest(t *testing.T) (*postgres.PostgresContainer, string, context.Context, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Start PostgreSQL container
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err, "failed to start PostgreSQL container")

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := pgContainer.Terminate(cleanupCtx); err != nil {
			t.Logf("Warning: failed to terminate PostgreSQL container: %v", err)
		}
		cancel()
	})

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	return pgContainer, connStr, ctx, cancel
}

func TestDatabase_PostgreSQL_Query(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	// Start agent first, then create collector to override tracer provider
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	// Open instrumented database connection
	db, err := database.Open(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NoError(t, err)
	defer db.Close()

	// Create a parent span
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-parent")
	defer parentSpan.End()

	// Execute a query
	rows, err := db.QueryContext(ctx, "SELECT 1 as num")
	require.NoError(t, err)
	defer rows.Close()

	// Consume results
	for rows.Next() {
		var num int
		err := rows.Scan(&num)
		require.NoError(t, err)
		assert.Equal(t, 1, num)
	}

	// Wait for spans
	time.Sleep(200 * time.Millisecond)

	// Verify spans
	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v)", i, span.Name(), span.SpanKind())
	}

	// Should have parent span + database span(s)
	require.GreaterOrEqual(t, len(spans), 2, "should have at least 2 spans")

	// Find database span
	dbSpan := testutil.FindSpanByKind(spans, trace.SpanKindClient)
	require.NotNil(t, dbSpan, "database client span not found")
}

func TestDatabase_PostgreSQL_Exec(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	db, err := database.Open(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NoError(t, err)
	defer db.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-exec-parent")
	defer parentSpan.End()

	// Create table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS test_users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert data
	result, err := db.ExecContext(ctx, "INSERT INTO test_users (name) VALUES ($1)", "Alice")
	require.NoError(t, err)

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "should have spans for CREATE and INSERT")
}

func TestDatabase_PostgreSQL_Transaction(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	db, err := database.Open(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NoError(t, err)
	defer db.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-tx-parent")
	defer parentSpan.End()

	// Create table first
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tx_test (
			id SERIAL PRIMARY KEY,
			value INT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Start transaction
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert within transaction
	_, err = tx.ExecContext(ctx, "INSERT INTO tx_test (value) VALUES ($1)", 42)
	require.NoError(t, err)

	// Commit transaction
	err = tx.Commit()
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	require.GreaterOrEqual(t, len(spans), 2, "should have transaction spans")
}

func TestDatabase_MustOpen_Success(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	// MustOpen should not panic with valid config
	db := database.MustOpen(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NotNil(t, db)
	defer db.Close()

	// Verify connection works
	err = db.Ping()
	require.NoError(t, err)
}

func TestDatabase_MustOpen_Panic(t *testing.T) {
	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	// MustOpen should panic with invalid driver (not just invalid DSN)
	assert.Panics(t, func() {
		database.MustOpen(database.Config{
			DriverName:   "nonexistent-driver",
			DSN:          "invalid",
			DatabaseName: "testdb",
		})
	})
}

func TestDatabase_QueryRow(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	db, err := database.Open(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NoError(t, err)
	defer db.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-queryrow-parent")
	defer parentSpan.End()

	// QueryRow
	var result int
	err = db.QueryRowContext(ctx, "SELECT 42").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 42, result)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "should have query span")
}

func TestDatabase_PreparedStatement(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	db, err := database.Open(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NoError(t, err)
	defer db.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-prepared-parent")
	defer parentSpan.End()

	// Prepare statement
	stmt, err := db.PrepareContext(ctx, "SELECT $1::int + $2::int")
	require.NoError(t, err)
	defer stmt.Close()

	// Execute prepared statement
	var result int
	err = stmt.QueryRowContext(ctx, 10, 20).Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 30, result)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "should have prepared statement spans")
}

func TestDatabase_ConnectionAttributes(t *testing.T) {
	_, connStr, ctx, cancel := setupPostgresTest(t)
	defer cancel()

	err := agent.Start()
	require.NoError(t, err)
	defer agent.Shutdown()

	collector := testutil.NewMockCollector()
	defer collector.Shutdown(ctx)

	db, err := database.Open(database.Config{
		DriverName:   "postgres",
		DSN:          connStr,
		DatabaseName: "testdb",
	})
	require.NoError(t, err)
	defer db.Close()

	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-attributes-parent")
	defer parentSpan.End()

	// Execute a simple query
	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	spans := collector.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "should have query span")

	// Find database span
	dbSpan := testutil.FindSpanByKind(spans, trace.SpanKindClient)
	require.NotNil(t, dbSpan, "database client span not found")

	// Verify connection attributes are present
	attrs := dbSpan.Attributes()
	attrMap := make(map[string]interface{})
	for _, attr := range attrs {
		attrMap[string(attr.Key)] = attr.Value.AsInterface()
	}

	t.Logf("Database span attributes: %+v", attrMap)

	// Verify server.address is set (replaces old net.peer.name)
	assert.Contains(t, attrMap, "server.address", "server.address attribute should be set")
	assert.NotEmpty(t, attrMap["server.address"], "server.address should not be empty")

	// Verify server.port is set
	assert.Contains(t, attrMap, "server.port", "server.port attribute should be set")

	// Verify db.user is set
	assert.Contains(t, attrMap, "db.user", "db.user attribute should be set")
	assert.Equal(t, "testuser", attrMap["db.user"], "db.user should match connection string")

	// Verify db.name is set
	assert.Contains(t, attrMap, "db.name", "db.name attribute should be set")
	assert.Equal(t, "testdb", attrMap["db.name"], "db.name should match database name")

	// Verify db.system is set
	assert.Contains(t, attrMap, "db.system", "db.system attribute should be set")
}
