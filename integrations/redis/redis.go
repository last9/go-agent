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
// Returns both the client and any instrumentation error.
// The client is always returned and usable even if instrumentation fails.
//
// Example usage:
//
//	rdb, err := redis.NewClient(&redis.Options{
//	    Addr:     "localhost:6379",
//	    Password: "", // no password
//	    DB:       0,  // default DB
//	})
//	if err != nil {
//	    log.Printf("Warning: Redis instrumentation failed: %v", err)
//	    // Client is still usable, just without instrumentation
//	}
//	defer rdb.Close()
//
//	// Use normally - commands are traced if instrumentation succeeded
//	err = rdb.Set(ctx, "key", "value", 0).Err()
func NewClient(opts *redis.Options) (*redis.Client, error) {
	client := redis.NewClient(opts)
	err := setupInstrumentation(client)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to instrument Redis client: %v (client still usable)", err)
	}
	return client, err
}

// NewClusterClient creates a new Redis cluster client with Last9 instrumentation.
// Returns both the client and any instrumentation error.
// The client is always returned and usable even if instrumentation fails.
//
// Example:
//
//	rdb, err := redis.NewClusterClient(&redis.ClusterOptions{
//	    Addrs: []string{":7000", ":7001", ":7002"},
//	})
//	if err != nil {
//	    log.Printf("Warning: Redis cluster instrumentation failed: %v", err)
//	}
//	defer rdb.Close()
func NewClusterClient(opts *redis.ClusterOptions) (*redis.ClusterClient, error) {
	client := redis.NewClusterClient(opts)
	err := setupClusterInstrumentation(client)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to instrument Redis cluster client: %v (client still usable)", err)
	}
	return client, err
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
