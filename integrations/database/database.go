// Package database provides instrumented database helpers for Last9
package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.nhat.io/otelsql"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

// Config holds database configuration
type Config struct {
	// DriverName is the SQL driver name (e.g., "postgres", "mysql")
	DriverName string

	// DSN is the data source name / connection string
	DSN string

	// DatabaseName is the name of the database (for telemetry)
	DatabaseName string

	// Additional otelsql driver options
	Options []otelsql.DriverOption
}

// extractDSNAttributes parses a database connection string and extracts
// OpenTelemetry semantic convention attributes for database spans.
//
// This function extracts:
//   - server.address: The database host (using semconv v1.21.0+)
//   - server.port: The database port (using semconv v1.21.0+)
//   - db.user: The database user
//   - db.name: The database name (can be overridden via Config.DatabaseName)
//
// Example PostgreSQL DSN: postgres://user:pass@localhost:5432/dbname?sslmode=disable
// Example MySQL DSN: user:pass@tcp(localhost:3306)/dbname
//
// Note: The older net.peer.name attribute has been replaced by server.address
// in OpenTelemetry semantic conventions v1.21.0+
func extractDSNAttributes(dsn, driverName string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}

	// Handle empty DSN
	if dsn == "" {
		return attrs
	}

	// Parse based on driver type
	switch driverName {
	case "postgres", "pgx":
		return extractPostgresDSNAttributes(dsn)
	case "mysql":
		return extractMySQLDSNAttributes(dsn)
	case "sqlite", "sqlite3":
		// SQLite uses file paths, no network attributes needed
		return attrs
	default:
		// Try parsing as URL for generic drivers
		return extractPostgresDSNAttributes(dsn)
	}
}

// extractPostgresDSNAttributes parses PostgreSQL-style connection strings
// Format: postgres://user:pass@host:port/dbname?params
func extractPostgresDSNAttributes(dsn string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}

	// Parse the connection URI
	parsedURI, err := url.Parse(dsn)
	if err != nil {
		// If parsing fails, return empty attributes rather than failing
		return attrs
	}

	// Extract host (server.address)
	if parsedURI.Hostname() != "" {
		attrs = append(attrs, semconv.ServerAddress(parsedURI.Hostname()))
	}

	// Extract port (server.port)
	if parsedURI.Port() != "" {
		if port, err := strconv.Atoi(parsedURI.Port()); err == nil {
			attrs = append(attrs, semconv.ServerPort(port))
		}
	}

	// Extract database user (db.user)
	if parsedURI.User != nil {
		if username := parsedURI.User.Username(); username != "" {
			attrs = append(attrs, semconv.DBUser(username))
		}
	}

	// Extract database name (db.name)
	// Note: This can be overridden by Config.DatabaseName
	if parsedURI.Path != "" {
		dbName := strings.TrimPrefix(parsedURI.Path, "/")
		if dbName != "" {
			attrs = append(attrs, semconv.DBName(dbName))
		}
	}

	return attrs
}

// extractMySQLDSNAttributes parses MySQL-style connection strings
// Format: user:pass@tcp(host:port)/dbname or user:pass@unix(/path/to/socket)/dbname
func extractMySQLDSNAttributes(dsn string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}

	// Extract user
	if idx := strings.Index(dsn, "@"); idx > 0 {
		userPass := dsn[:idx]
		if colonIdx := strings.Index(userPass, ":"); colonIdx > 0 {
			username := userPass[:colonIdx]
			attrs = append(attrs, semconv.DBUser(username))
		} else {
			attrs = append(attrs, semconv.DBUser(userPass))
		}
		dsn = dsn[idx+1:] // Remove user part
	}

	// Extract host and port from tcp(host:port) format
	if strings.HasPrefix(dsn, "tcp(") {
		endIdx := strings.Index(dsn, ")")
		if endIdx > 0 {
			hostPort := dsn[4:endIdx]
			if colonIdx := strings.LastIndex(hostPort, ":"); colonIdx > 0 {
				host := hostPort[:colonIdx]
				port := hostPort[colonIdx+1:]
				attrs = append(attrs, semconv.ServerAddress(host))
				if portNum, err := strconv.Atoi(port); err == nil {
					attrs = append(attrs, semconv.ServerPort(portNum))
				}
			} else {
				attrs = append(attrs, semconv.ServerAddress(hostPort))
			}
			dsn = dsn[endIdx+1:] // Remove tcp(...) part
		}
	}

	// Extract database name
	if strings.HasPrefix(dsn, "/") {
		dbName := dsn[1:]
		if qIdx := strings.Index(dbName, "?"); qIdx > 0 {
			dbName = dbName[:qIdx]
		}
		if dbName != "" {
			attrs = append(attrs, semconv.DBName(dbName))
		}
	}

	return attrs
}

// Open opens a database connection with Last9 instrumentation.
// It wraps the standard sql.Open with automatic tracing and metrics.
//
// Traces collected:
//   - Database query execution with SQL statements
//   - Connection lifecycle events
//   - Transaction operations
//
// Metrics collected:
//   - db.client.connections.usage (gauge) - Number of connections in use
//   - db.client.connections.idle (gauge) - Number of idle connections
//   - db.client.connections.max (gauge) - Maximum number of open connections
//   - db.client.connections.wait_time (histogram) - Time to acquire a connection
//   - db.client.connections.use_time (histogram) - Time connections are in use
//   - db.client.connections.idle_time (histogram) - Time connections are idle
//
// Example usage:
//
//	db, err := database.Open(database.Config{
//	    DriverName:   "postgres",
//	    DSN:          "postgres://user:pass@localhost/mydb?sslmode=disable",
//	    DatabaseName: "mydb",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
func Open(cfg Config) (*sql.DB, error) {
	// Validate inputs
	if cfg.DriverName == "" {
		return nil, fmt.Errorf("database.Open: DriverName is required")
	}
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database.Open: DSN is required")
	}

	// Extract connection attributes from DSN
	// This adds server.address, server.port, db.user, and db.name from the connection string
	dsnAttrs := extractDSNAttributes(cfg.DSN, cfg.DriverName)

	// Default driver options
	opts := []otelsql.DriverOption{
		otelsql.AllowRoot(),
		otelsql.TraceQueryWithoutArgs(),
		otelsql.TraceRowsClose(),
		otelsql.TraceRowsAffected(),
	}

	// Add DSN-extracted attributes
	if len(dsnAttrs) > 0 {
		opts = append(opts, otelsql.WithDefaultAttributes(dsnAttrs...))
	}

	// Add database name if provided (this will override db.name from DSN if different)
	if cfg.DatabaseName != "" {
		opts = append(opts, otelsql.WithDatabaseName(cfg.DatabaseName))
	}

	// Add system semantic convention based on driver
	switch cfg.DriverName {
	case "postgres", "pgx":
		opts = append(opts, otelsql.WithSystem(semconv.DBSystemPostgreSQL))
	case "mysql":
		opts = append(opts, otelsql.WithSystem(semconv.DBSystemMySQL))
	case "sqlite", "sqlite3":
		opts = append(opts, otelsql.WithSystem(semconv.DBSystemSqlite))
	}

	// Append any custom options
	opts = append(opts, cfg.Options...)

	// Register instrumented driver
	driverName, err := otelsql.Register(cfg.DriverName, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to register instrumented driver: %w", err)
	}

	// Open database connection
	db, err := sql.Open(driverName, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Record stats for metrics
	if err := otelsql.RecordStats(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to record stats: %w", err)
	}

	return db, nil
}

// MustOpen is like Open but panics on error.
// Useful for initialization code where database connection is critical.
//
// Example:
//
//	db := database.MustOpen(database.Config{
//	    DriverName:   "postgres",
//	    DSN:          os.Getenv("DATABASE_URL"),
//	    DatabaseName: "mydb",
//	})
//	defer db.Close()
func MustOpen(cfg Config) *sql.DB {
	db, err := Open(cfg)
	if err != nil {
		panic(fmt.Sprintf("database.MustOpen: %v", err))
	}
	return db
}
