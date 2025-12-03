// Package redis provides instrumented Redis client helpers for Last9
package redis

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// NewClient creates a new Redis client with Last9 instrumentation.
// It's a drop-in replacement for redis.NewClient() with automatic tracing.
//
// Example usage:
//
//	rdb := redis.NewClient(&redis.Options{
//	    Addr:     "localhost:6379",
//	    Password: "", // no password
//	    DB:       0,  // default DB
//	})
//	defer rdb.Close()
//
//	// Use normally - all commands are automatically traced
//	err := rdb.Set(ctx, "key", "value", 0).Err()
func NewClient(opts *redis.Options) *redis.Client {
	client := redis.NewClient(opts)
	setupInstrumentation(client)
	return client
}

// NewClusterClient creates a new Redis cluster client with Last9 instrumentation.
//
// Example:
//
//	rdb := redis.NewClusterClient(&redis.ClusterOptions{
//	    Addrs: []string{":7000", ":7001", ":7002"},
//	})
//	defer rdb.Close()
func NewClusterClient(opts *redis.ClusterOptions) *redis.ClusterClient {
	client := redis.NewClusterClient(opts)
	setupClusterInstrumentation(client)
	return client
}

// Instrument adds Last9 instrumentation to an existing Redis client.
// Use this if you need to instrument a client created elsewhere.
//
// Example:
//
//	rdb := redis.NewClient(opts)
//	redis.Instrument(rdb)
func Instrument(client *redis.Client) error {
	return setupInstrumentation(client)
}

// InstrumentCluster adds Last9 instrumentation to an existing Redis cluster client.
func InstrumentCluster(client *redis.ClusterClient) error {
	return setupClusterInstrumentation(client)
}

// setupInstrumentation configures tracing for a Redis client
func setupInstrumentation(client *redis.Client) error {
	if err := redisotel.InstrumentTracing(client); err != nil {
		return fmt.Errorf("failed to instrument Redis tracing: %w", err)
	}

	// Optional: instrument metrics
	if err := redisotel.InstrumentMetrics(client); err != nil {
		log.Printf("[Last9 Agent] Warning: failed to instrument Redis metrics: %v", err)
	}

	return nil
}

// setupClusterInstrumentation configures tracing for a Redis cluster client
func setupClusterInstrumentation(client *redis.ClusterClient) error {
	if err := redisotel.InstrumentTracing(client); err != nil {
		return fmt.Errorf("failed to instrument Redis cluster tracing: %w", err)
	}

	// Optional: instrument metrics
	if err := redisotel.InstrumentMetrics(client); err != nil {
		log.Printf("[Last9 Agent] Warning: failed to instrument Redis cluster metrics: %v", err)
	}

	return nil
}

// Ping tests the Redis connection
func Ping(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
