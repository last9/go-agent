// Package mongodb provides OpenTelemetry instrumentation for MongoDB using
// the mongo-driver v1 event.CommandMonitor API.
//
// The official otelmongo contrib package has been deprecated and removed.
// This package re-implements MongoDB instrumentation directly using the
// driver's built-in CommandMonitor hook.
//
// Usage — new client:
//
//	client, err := mongodb.NewClient(mongodb.Config{
//	    URI: "mongodb://localhost:27017/mydb",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Disconnect(context.Background())
//
// Usage — instrument existing options before Connect:
//
//	opts := options.Client().ApplyURI("mongodb://localhost:27017")
//	client, err := mongodb.Instrument(opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Disconnect(context.Background())
package mongodb

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/last9/go-agent"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/last9/go-agent/integrations/mongodb"

// Config holds MongoDB client configuration for instrumented client creation.
type Config struct {
	// ClientOptions allows passing additional mongo driver options.
	// These are merged on top of options derived from the URI.
	ClientOptions *options.ClientOptions

	// URI is the MongoDB connection string.
	// Example: "mongodb://user:pass@localhost:27017/mydb"
	URI string
}

// defaultSkippedCommands contains housekeeping/auth commands that produce
// noise without useful application-level tracing information.
var defaultSkippedCommands = map[string]struct{}{
	"hello":        {},
	"isMaster":     {},
	"ping":         {},
	"saslStart":    {},
	"saslContinue": {},
	"getnonce":     {},
	"authenticate": {},
	"endSessions":  {},
}

// spanKey uniquely identifies an in-flight MongoDB command.
// connectionID + requestID is guaranteed unique per concurrent command.
type spanKey struct {
	connectionID string
	requestID    int64
}

// spanEntry stores an open span for later retrieval in Succeeded/Failed callbacks.
type spanEntry struct {
	span trace.Span
}

// monitor holds the OTel instrumentation state for a MongoDB client.
type monitor struct {
	tracer         trace.Tracer
	spans          sync.Map // map[spanKey]spanEntry
	operationCount metric.Int64Counter
	errorCount     metric.Int64Counter
	duration       metric.Float64Histogram
	baseAttrs      []attribute.KeyValue
}

// NewClient creates a new instrumented mongo.Client.
// The Last9 agent is started automatically if not already initialized.
//
// The returned client has tracing and metrics enabled for all database commands.
// Always call client.Disconnect(ctx) when done.
//
// Example:
//
//	client, err := mongodb.NewClient(mongodb.Config{
//	    URI: "mongodb://localhost:27017/mydb",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Disconnect(context.Background())
func NewClient(cfg Config) (*mongo.Client, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("mongodb.NewClient: URI is required")
	}

	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for MongoDB instrumentation: %v", err)
		}
	}

	opts := options.Client().ApplyURI(cfg.URI)
	if cfg.ClientOptions != nil {
		opts = options.MergeClientOptions(opts, cfg.ClientOptions)
	}

	baseAttrs := extractURIAttributes(cfg.URI)
	return connectWithMonitor(opts, baseAttrs)
}

// Instrument injects tracing into existing *options.ClientOptions and returns
// an instrumented, connected client.
//
// Because the mongo-driver v1 CommandMonitor must be set before the client is
// created (it cannot be added to a live *mongo.Client), Instrument accepts
// pre-connection options rather than a connected client.
//
// Example:
//
//	opts := options.Client().ApplyURI(os.Getenv("MONGO_URI"))
//	opts.SetAuth(options.Credential{Username: "admin", Password: "secret"})
//	client, err := mongodb.Instrument(opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Disconnect(context.Background())
func Instrument(opts *options.ClientOptions) (*mongo.Client, error) {
	if opts == nil {
		return nil, fmt.Errorf("mongodb.Instrument: opts must not be nil")
	}

	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for MongoDB instrumentation: %v", err)
		}
	}

	var baseAttrs []attribute.KeyValue
	if len(opts.Hosts) > 0 {
		baseAttrs = extractHostAttributes(opts.Hosts)
	}

	return connectWithMonitor(opts, baseAttrs)
}

// connectWithMonitor creates the OTel monitor, injects it into the client
// options, and calls mongo.Connect.
func connectWithMonitor(opts *options.ClientOptions, baseAttrs []attribute.KeyValue) (*mongo.Client, error) {
	m, monitorErr := newMonitor(baseAttrs)
	if monitorErr != nil {
		log.Printf("[Last9 Agent] Warning: partial MongoDB monitor setup: %v", monitorErr)
	}

	// If monitor creation failed entirely, connect without instrumentation.
	if m != nil {
		cmdMonitor := m.commandMonitor()

		// Chain with any existing monitor the caller may have set.
		if opts.Monitor != nil {
			cmdMonitor = chainMonitors(opts.Monitor, cmdMonitor)
		}
		opts.SetMonitor(cmdMonitor)
	}

	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb: failed to connect: %w", err)
	}
	// Return the client even if monitor setup partially failed, but surface
	// the error so callers can decide how to handle it (matching Redis pattern).
	return client, monitorErr
}

// newMonitor creates a monitor with OTel tracer and metric instruments.
func newMonitor(baseAttrs []attribute.KeyValue) (*monitor, error) {
	meter := otel.Meter(instrumentationName)
	var firstErr error

	operationCount, err := meter.Int64Counter(
		"db.mongodb.operations",
		metric.WithDescription("Number of MongoDB operations executed"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create MongoDB operations counter: %v", err)
		firstErr = err
	}

	errorCount, err := meter.Int64Counter(
		"db.mongodb.errors",
		metric.WithDescription("Number of failed MongoDB operations"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create MongoDB errors counter: %v", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	dur, err := meter.Float64Histogram(
		"db.mongodb.operation.duration",
		metric.WithDescription("Duration of MongoDB operations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create MongoDB duration histogram: %v", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	return &monitor{
		tracer:         otel.Tracer(instrumentationName),
		operationCount: operationCount,
		errorCount:     errorCount,
		duration:       dur,
		baseAttrs:      baseAttrs,
	}, firstErr
}

// commandMonitor returns an *event.CommandMonitor that creates and manages
// OTel spans for MongoDB commands.
func (m *monitor) commandMonitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started:   m.started,
		Succeeded: m.succeeded,
		Failed:    m.failed,
	}
}

// started handles CommandStartedEvent: creates a span and stores it in the registry.
func (m *monitor) started(ctx context.Context, evt *event.CommandStartedEvent) {
	if _, skip := defaultSkippedCommands[evt.CommandName]; skip {
		return
	}

	collection := extractCollectionName(evt.CommandName, evt.Command)

	// Span name: "{operation} {collection}" per OTel DB semconv
	spanName := evt.CommandName
	if collection != "" {
		spanName = evt.CommandName + " " + collection
	}

	attrs := []attribute.KeyValue{
		semconv.DBSystemMongoDB,
		semconv.DBName(evt.DatabaseName),
		semconv.DBOperation(evt.CommandName),
	}
	if collection != "" {
		attrs = append(attrs, semconv.DBMongoDBCollection(collection))
	}
	attrs = append(attrs, m.baseAttrs...)

	_, span := m.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)

	m.spans.Store(
		spanKey{evt.ConnectionID, evt.RequestID},
		spanEntry{span: span},
	)
}

// succeeded handles CommandSucceededEvent: ends the span successfully.
func (m *monitor) succeeded(ctx context.Context, evt *event.CommandSucceededEvent) {
	key := spanKey{evt.ConnectionID, evt.RequestID}
	raw, ok := m.spans.LoadAndDelete(key)
	if !ok {
		return
	}
	entry, ok := raw.(spanEntry)
	if !ok {
		return
	}
	entry.span.End()
	m.recordMetrics(ctx, evt.CommandName, false, evt.Duration)
}

// failed handles CommandFailedEvent: records the error and ends the span.
func (m *monitor) failed(ctx context.Context, evt *event.CommandFailedEvent) {
	key := spanKey{evt.ConnectionID, evt.RequestID}
	raw, ok := m.spans.LoadAndDelete(key)
	if !ok {
		return
	}
	entry, ok := raw.(spanEntry)
	if !ok {
		return
	}
	entry.span.RecordError(fmt.Errorf("%s", evt.Failure))
	entry.span.SetStatus(codes.Error, evt.Failure)
	entry.span.End()
	m.recordMetrics(ctx, evt.CommandName, true, evt.Duration)
}

// recordMetrics records operation count and duration.
func (m *monitor) recordMetrics(ctx context.Context, operation string, isError bool, elapsed time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "mongodb"),
		attribute.String("db.operation", operation),
	}
	attrs = append(attrs, m.baseAttrs...)
	metricOpts := metric.WithAttributes(attrs...)

	durationMS := float64(elapsed.Nanoseconds()) / 1e6

	if m.operationCount != nil {
		m.operationCount.Add(ctx, 1, metricOpts)
	}
	if isError && m.errorCount != nil {
		m.errorCount.Add(ctx, 1, metricOpts)
	}
	if m.duration != nil {
		m.duration.Record(ctx, durationMS, metricOpts)
	}
}

// extractCollectionName reads the collection name from a BSON command document.
// Most MongoDB commands encode the collection as the value of the key matching
// the command name: {"find": "users", "filter": {...}}.
func extractCollectionName(commandName string, command bson.Raw) string {
	if len(command) == 0 {
		return ""
	}

	// Special case: getMore stores collection under "collection" key
	if commandName == "getMore" {
		val, err := command.LookupErr("collection")
		if err != nil {
			return ""
		}
		collection, ok := val.StringValueOK()
		if !ok {
			return ""
		}
		return collection
	}

	val, err := command.LookupErr(commandName)
	if err != nil {
		return ""
	}
	collection, ok := val.StringValueOK()
	if !ok || collection == "" {
		return ""
	}
	// Filter out internal targets
	if collection == "1" || collection == "$cmd" {
		return ""
	}
	return collection
}

// extractURIAttributes parses a MongoDB connection URI and returns OTel
// semantic convention attributes.
//
// Supported formats:
//
//	mongodb://host:27017/dbname
//	mongodb+srv://host/dbname
//	mongodb://user:pass@host1:27017,host2:27017/dbname (uses first host)
func extractURIAttributes(uri string) []attribute.KeyValue {
	if uri == "" {
		return nil
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil
	}

	var attrs []attribute.KeyValue

	// Extract user
	if parsed.User != nil {
		if username := parsed.User.Username(); username != "" {
			attrs = append(attrs, semconv.DBUser(username))
		}
	}

	// Extract first host (replica sets list multiple comma-separated hosts)
	hostPart := parsed.Host
	if idx := strings.Index(hostPart, ","); idx != -1 {
		hostPart = hostPart[:idx]
	}

	host, portStr := splitHostPort(hostPart)
	if host != "" {
		attrs = append(attrs, semconv.ServerAddress(host))
	}
	if portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			attrs = append(attrs, semconv.ServerPort(port))
		}
	} else if host != "" && parsed.Scheme == "mongodb" {
		attrs = append(attrs, semconv.ServerPort(27017))
	}

	// Note: db.name is not extracted from the URI here because it is set
	// per-operation from evt.DatabaseName in the started() callback. This avoids
	// duplicate attributes when the application uses multiple databases.

	return attrs
}

// extractHostAttributes extracts server attributes from a list of host strings
// (e.g., ["localhost:27017"]). Uses only the first host.
func extractHostAttributes(hosts []string) []attribute.KeyValue {
	if len(hosts) == 0 {
		return nil
	}
	var attrs []attribute.KeyValue
	host, portStr := splitHostPort(hosts[0])
	if host != "" {
		attrs = append(attrs, semconv.ServerAddress(host))
	}
	if portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			attrs = append(attrs, semconv.ServerPort(port))
		}
	}
	return attrs
}

// splitHostPort splits a host:port string, handling IPv6 addresses.
func splitHostPort(hostport string) (host, port string) {
	if hostport == "" {
		return "", ""
	}
	// IPv6: [::1]:27017
	if strings.HasPrefix(hostport, "[") {
		end := strings.LastIndex(hostport, "]")
		if end == -1 {
			return hostport, ""
		}
		host = hostport[1:end]
		rest := hostport[end+1:]
		if strings.HasPrefix(rest, ":") {
			port = rest[1:]
		}
		return host, port
	}
	if idx := strings.LastIndex(hostport, ":"); idx != -1 {
		return hostport[:idx], hostport[idx+1:]
	}
	return hostport, ""
}

// chainMonitors creates a CommandMonitor that calls both a and b for each event.
func chainMonitors(a, b *event.CommandMonitor) *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(ctx context.Context, evt *event.CommandStartedEvent) {
			if a.Started != nil {
				a.Started(ctx, evt)
			}
			if b.Started != nil {
				b.Started(ctx, evt)
			}
		},
		Succeeded: func(ctx context.Context, evt *event.CommandSucceededEvent) {
			if a.Succeeded != nil {
				a.Succeeded(ctx, evt)
			}
			if b.Succeeded != nil {
				b.Succeeded(ctx, evt)
			}
		},
		Failed: func(ctx context.Context, evt *event.CommandFailedEvent) {
			if a.Failed != nil {
				a.Failed(ctx, evt)
			}
			if b.Failed != nil {
				b.Failed(ctx, evt)
			}
		},
	}
}
