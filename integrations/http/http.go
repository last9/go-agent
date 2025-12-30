// Package http provides instrumented HTTP client helpers for Last9
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultClient returns a new HTTP client with Last9 instrumentation.
// It's a drop-in replacement for http.DefaultClient with automatic tracing.
//
// Example usage:
//
//	client := http.DefaultClient()
//	resp, err := client.Get("https://api.example.com/data")
var DefaultClient = NewClient(nil)

// NewClient creates a new HTTP client with Last9 instrumentation.
// If client is nil, it uses http.DefaultTransport.
// Automatically captures traces and metrics for all HTTP requests.
//
// Metrics collected:
//   - http.client.request.duration (histogram)
//   - http.client.request.body.size (histogram)
//   - http.client.response.body.size (histogram)
//
// Example:
//
//	client := http.NewClient(&http.Client{
//	    Timeout: 10 * time.Second,
//	})
//	resp, err := client.Get("https://api.example.com/data")
func NewClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}

	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}

	// Wrap transport with OpenTelemetry instrumentation (includes metrics by default)
	client.Transport = otelhttp.NewTransport(
		client.Transport,
		otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
			return otelhttptrace.NewClientTrace(ctx)
		}),
	)

	return client
}

// Get is a convenience function for making instrumented GET requests.
//
// Example:
//
//	resp, err := http.Get(ctx, "https://api.example.com/data")
func Get(ctx context.Context, url string) (*http.Response, error) {
	if url == "" {
		return nil, fmt.Errorf("http.Get: url is required")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return DefaultClient.Do(req)
}

// Post is a convenience function for making instrumented POST requests.
// It supports multiple body types: io.Reader, []byte, string, and structs (JSON encoded).
//
// Example with JSON:
//
//	type User struct { Name string `json:"name"` }
//	user := User{Name: "Alice"}
//	resp, err := http.Post(ctx, "https://api.example.com/users", "application/json", user)
//
// Example with raw data:
//
//	resp, err := http.Post(ctx, "https://api.example.com/data", "text/plain", []byte("hello"))
func Post(ctx context.Context, url, contentType string, body interface{}) (*http.Response, error) {
	// Validate inputs
	if url == "" {
		return nil, fmt.Errorf("http.Post: url is required")
	}
	if contentType == "" {
		return nil, fmt.Errorf("http.Post: contentType is required")
	}

	var bodyReader io.Reader

	// Handle different body types
	switch v := body.(type) {
	case io.Reader:
		bodyReader = v
	case []byte:
		bodyReader = bytes.NewReader(v)
	case string:
		bodyReader = strings.NewReader(v)
	case nil:
		bodyReader = nil
	default:
		// For structs/maps, encode as JSON if content type is JSON
		if strings.Contains(contentType, "application/json") {
			jsonData, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body to JSON: %w", err)
			}
			bodyReader = bytes.NewReader(jsonData)
		} else {
			return nil, fmt.Errorf("unsupported body type %T for content-type %s", body, contentType)
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return DefaultClient.Do(req)
}
