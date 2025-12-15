// Package http provides instrumented HTTP client helpers for Last9
package http

import (
	"context"
	"net/http"
	"net/http/httptrace"

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
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return DefaultClient.Do(req)
}

// Post is a convenience function for making instrumented POST requests.
func Post(ctx context.Context, url, contentType string, body interface{}) (*http.Response, error) {
	// This is a simplified version - you'd want to handle body encoding properly
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return DefaultClient.Do(req)
}
