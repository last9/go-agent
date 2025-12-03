// Package database provides instrumented database helpers for Last9
package database

import (
	"database/sql"
	"fmt"

	"go.nhat.io/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds database configuration
type Config struct {
	// DriverName is the SQL driver name (e.g., "postgres", "mysql")
	DriverName string

	// DSN is the data source name / connection string
	DSN string

	// DatabaseName is the name of the database (for telemetry)
	DatabaseName string

	// Additional otelsql options
	Options []otelsql.Option
}

// Open opens a database connection with Last9 instrumentation.
// It wraps the standard sql.Open with automatic tracing and metrics.
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
	// Default options
	opts := []otelsql.Option{
		otelsql.AllowRoot(),
		otelsql.TraceQueryWithoutArgs(),
		otelsql.TraceRowsClose(),
		otelsql.TraceRowsAffected(),
	}

	// Add database name if provided
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
