package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"

	"github.com/last9/go-agent/integrations/database"
)

func TestParseDSNAttributes_Postgres(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		wantAddress   string
		wantUser      string
		wantDB        string
		wantPort      int
		wantAttrCount int
	}{
		{
			name:          "full URL with port",
			dsn:           "postgres://myuser:mypass@db.example.com:5432/mydb?sslmode=disable",
			wantAddress:   "db.example.com",
			wantPort:      5432,
			wantUser:      "myuser",
			wantDB:        "mydb",
			wantAttrCount: 4,
		},
		{
			name:          "localhost without port",
			dsn:           "postgres://user:pass@localhost/testdb",
			wantAddress:   "localhost",
			wantPort:      0,
			wantUser:      "user",
			wantDB:        "testdb",
			wantAttrCount: 3,
		},
		{
			name:          "pgx driver treated same as postgres",
			dsn:           "postgres://admin:secret@10.0.0.1:5433/appdb",
			wantAddress:   "10.0.0.1",
			wantPort:      5433,
			wantUser:      "admin",
			wantDB:        "appdb",
			wantAttrCount: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attrs := database.ParseDSNAttributes(tc.dsn, "postgres")
			assert.Len(t, attrs, tc.wantAttrCount)

			attrMap := kvsToMap(attrs)

			assert.Equal(t, tc.wantAddress, attrMap[string(semconv.ServerAddressKey)])
			if tc.wantPort != 0 {
				assert.Equal(t, int64(tc.wantPort), attrMap[string(semconv.ServerPortKey)])
			}
			assert.Equal(t, tc.wantUser, attrMap[string(semconv.DBUserKey)])
			assert.Equal(t, tc.wantDB, attrMap[string(semconv.DBNameKey)])
		})
	}
}

func TestParseDSNAttributes_MySQL(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		wantAddress   string
		wantUser      string
		wantDB        string
		wantPort      int
		wantAttrCount int
	}{
		{
			name:          "standard tcp DSN with port",
			dsn:           "myuser:mypass@tcp(db.example.com:3306)/mydb",
			wantAddress:   "db.example.com",
			wantPort:      3306,
			wantUser:      "myuser",
			wantDB:        "mydb",
			wantAttrCount: 4,
		},
		{
			name:          "tcp DSN without port",
			dsn:           "user:pass@tcp(localhost)/testdb",
			wantAddress:   "localhost",
			wantPort:      0,
			wantUser:      "user",
			wantDB:        "testdb",
			wantAttrCount: 3,
		},
		{
			name:          "DSN with query params",
			dsn:           "root:secret@tcp(10.0.0.1:3307)/appdb?charset=utf8mb4&parseTime=True",
			wantAddress:   "10.0.0.1",
			wantPort:      3307,
			wantUser:      "root",
			wantDB:        "appdb",
			wantAttrCount: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attrs := database.ParseDSNAttributes(tc.dsn, "mysql")
			assert.Len(t, attrs, tc.wantAttrCount)

			attrMap := kvsToMap(attrs)

			assert.Equal(t, tc.wantAddress, attrMap[string(semconv.ServerAddressKey)])
			if tc.wantPort != 0 {
				assert.Equal(t, int64(tc.wantPort), attrMap[string(semconv.ServerPortKey)])
			}
			assert.Equal(t, tc.wantUser, attrMap[string(semconv.DBUserKey)])
			assert.Equal(t, tc.wantDB, attrMap[string(semconv.DBNameKey)])
		})
	}
}

func TestParseDSNAttributes_SQLite(t *testing.T) {
	attrs := database.ParseDSNAttributes("/path/to/db.sqlite", "sqlite3")
	assert.Empty(t, attrs, "SQLite should return no network attributes")
}

func TestParseDSNAttributes_EmptyDSN(t *testing.T) {
	attrs := database.ParseDSNAttributes("", "mysql")
	assert.Empty(t, attrs, "empty DSN should return no attributes")
}

func TestParseDSNAttributes_PGX(t *testing.T) {
	// pgx uses the same URL format as postgres
	attrs := database.ParseDSNAttributes("postgres://pguser:pgpass@pghost:5432/pgdb", "pgx")
	attrMap := kvsToMap(attrs)

	assert.Equal(t, "pghost", attrMap[string(semconv.ServerAddressKey)])
	assert.Equal(t, int64(5432), attrMap[string(semconv.ServerPortKey)])
	assert.Equal(t, "pguser", attrMap[string(semconv.DBUserKey)])
	assert.Equal(t, "pgdb", attrMap[string(semconv.DBNameKey)])
}

func TestParseDSNAttributes_UnknownDriver(t *testing.T) {
	// Unknown drivers fall back to URL parsing (postgres-style)
	attrs := database.ParseDSNAttributes("somedb://user:pass@host:1234/db", "somedb")
	attrMap := kvsToMap(attrs)

	assert.Equal(t, "host", attrMap[string(semconv.ServerAddressKey)])
	assert.Equal(t, int64(1234), attrMap[string(semconv.ServerPortKey)])
	assert.Equal(t, "user", attrMap[string(semconv.DBUserKey)])
	assert.Equal(t, "db", attrMap[string(semconv.DBNameKey)])
}

func TestParseDSNAttributes_MalformedDSN(t *testing.T) {
	// Malformed DSNs should return empty rather than panic
	attrs := database.ParseDSNAttributes("not a valid dsn ://!!", "postgres")
	assert.NotNil(t, attrs, "should return non-nil slice for malformed DSN")
}

func TestParseDSNAttributes_MySQLUnixSocket(t *testing.T) {
	// Unix socket DSNs have no tcp(...) block — no server.address/port expected.
	// db.name is also not extracted since the parser only handles tcp(...) format.
	attrs := database.ParseDSNAttributes("user:pass@unix(/var/run/mysqld/mysqld.sock)/mydb", "mysql")
	attrMap := kvsToMap(attrs)

	assert.Equal(t, "user", attrMap[string(semconv.DBUserKey)])
	assert.NotContains(t, attrMap, string(semconv.ServerAddressKey))
	assert.NotContains(t, attrMap, string(semconv.ServerPortKey))
}

// ExampleParseDSNAttributes demonstrates stamping connection attributes onto a
// manually created wrapper span. Without this, server.address and server.port
// only appear on spans auto-generated by the otelsql driver layer.
func ExampleParseDSNAttributes() {
	dsn := "user:pass@tcp(db.example.com:3306)/myapp"

	attrs := database.ParseDSNAttributes(dsn, "mysql")
	for _, attr := range attrs {
		_ = attr // span.SetAttributes(attrs...)
	}
	// Use: span.SetAttributes(database.ParseDSNAttributes(dsn, "mysql")...)
}

// kvsToMap converts []attribute.KeyValue to a plain map for easy assertions.
func kvsToMap(kvs []attribute.KeyValue) map[string]interface{} {
	m := make(map[string]interface{}, len(kvs))
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}
