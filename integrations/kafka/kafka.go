// Package kafka provides OpenTelemetry instrumentation for Kafka using IBM Sarama.
// It automatically traces Kafka producer sends and consumer receives with proper
// context propagation for distributed tracing.
//
// This integration uses IBM Sarama (formerly Shopify Sarama) which is the recommended
// Kafka client library for Go.
package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/IBM/sarama"
	"github.com/last9/go-agent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	instrumentationName = "github.com/last9/go-agent/integrations/kafka"
)

// ProducerConfig holds configuration for creating an instrumented Kafka producer.
type ProducerConfig struct {
	// Config is the Sarama configuration (optional)
	// If nil, a default configuration will be used
	Config *sarama.Config

	// Brokers is the list of Kafka broker addresses
	Brokers []string
}

// ConsumerConfig holds configuration for creating an instrumented Kafka consumer.
type ConsumerConfig struct {
	// Config is the Sarama configuration (optional)
	// If nil, a default configuration will be used
	Config *sarama.Config

	// Brokers is the list of Kafka broker addresses
	Brokers []string

	// GroupID is the consumer group ID
	GroupID string

	// Topics is the list of topics to subscribe to
	Topics []string
}

// SyncProducer wraps sarama.SyncProducer with OpenTelemetry tracing and metrics.
type SyncProducer struct {
	producer         sarama.SyncProducer
	tracer           trace.Tracer
	meter            metric.Meter
	messagesSent     metric.Int64Counter
	messageErrors    metric.Int64Counter
	sendDuration     metric.Float64Histogram
	messageSizeBytes metric.Int64Histogram
}

// NewSyncProducer creates a new synchronous Kafka producer with automatic OpenTelemetry tracing.
// All messages sent through this producer will be traced automatically.
//
// Example:
//
//	producer, err := kafka.NewSyncProducer(kafka.ProducerConfig{
//	    Brokers: []string{"localhost:9092"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer producer.Close()
//
//	// Send message (automatically traced)
//	partition, offset, err := producer.SendMessage(ctx, &sarama.ProducerMessage{
//	    Topic: "my-topic",
//	    Value: sarama.StringEncoder("Hello Kafka"),
//	})
func NewSyncProducer(cfg ProducerConfig) (*SyncProducer, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize Last9 agent: %w", err)
		}
	}

	// Use default config if not provided
	config := cfg.Config
	if config == nil {
		config = sarama.NewConfig()
		config.Producer.Return.Successes = true
		config.Version = sarama.V2_6_0_0
	}

	// Create base producer
	producer, err := sarama.NewSyncProducer(cfg.Brokers, config)
	if err != nil {
		return nil, err
	}

	// Create meter and metrics
	meter := otel.Meter(instrumentationName)

	messagesSent, err := meter.Int64Counter(
		"messaging.kafka.messages.sent",
		metric.WithDescription("Number of messages successfully sent to Kafka"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create messages_sent counter: %v", err)
	}

	messageErrors, err := meter.Int64Counter(
		"messaging.kafka.messages.errors",
		metric.WithDescription("Number of message send errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create message_errors counter: %v", err)
	}

	sendDuration, err := meter.Float64Histogram(
		"messaging.kafka.send.duration",
		metric.WithDescription("Duration of Kafka message send operations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create send_duration histogram: %v", err)
	}

	messageSizeBytes, err := meter.Int64Histogram(
		"messaging.kafka.message.size",
		metric.WithDescription("Size of Kafka messages in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create message_size histogram: %v", err)
	}

	return &SyncProducer{
		producer:         producer,
		tracer:           otel.Tracer(instrumentationName),
		meter:            meter,
		messagesSent:     messagesSent,
		messageErrors:    messageErrors,
		sendDuration:     sendDuration,
		messageSizeBytes: messageSizeBytes,
	}, nil
}

// SendMessage sends a message to Kafka with automatic tracing and metrics.
// Context is used for trace propagation.
func (p *SyncProducer) SendMessage(ctx context.Context, msg *sarama.ProducerMessage) (partition int32, offset int64, err error) {
	startTime := time.Now()

	// Start span
	ctx, span := p.tracer.Start(ctx, fmt.Sprintf("%s send", msg.Topic),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.MessagingSystemKey.String("kafka"),
			semconv.MessagingDestinationNameKey.String(msg.Topic),
			semconv.MessagingOperationNameKey.String("send"),
		),
	)
	defer span.End()

	// Inject trace context into message headers
	propagator := otel.GetTextMapPropagator()
	carrier := &producerMessageCarrier{msg: msg}
	propagator.Inject(ctx, carrier)

	// Calculate message size
	var msgSize int64
	if msg.Value != nil {
		msgSize = int64(msg.Value.Length())
	}

	// Send message
	partition, offset, err = p.producer.SendMessage(msg)

	// Record duration
	duration := time.Since(startTime).Milliseconds()
	metricAttrs := []attribute.KeyValue{
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", msg.Topic),
	}

	// Record result
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		// Record error metric
		if p.messageErrors != nil {
			p.messageErrors.Add(ctx, 1, metric.WithAttributes(metricAttrs...))
		}
	} else {
		span.SetAttributes(
			attribute.Int("messaging.kafka.partition", int(partition)),
			attribute.Int64("messaging.kafka.offset", offset),
		)

		// Record success metrics
		if p.messagesSent != nil {
			p.messagesSent.Add(ctx, 1, metric.WithAttributes(metricAttrs...))
		}
	}

	// Record duration and size regardless of success/failure
	if p.sendDuration != nil {
		p.sendDuration.Record(ctx, float64(duration), metric.WithAttributes(metricAttrs...))
	}
	if p.messageSizeBytes != nil && msgSize > 0 {
		p.messageSizeBytes.Record(ctx, msgSize, metric.WithAttributes(metricAttrs...))
	}

	return partition, offset, err
}

// SendMessages sends multiple messages to Kafka with automatic tracing.
func (p *SyncProducer) SendMessages(ctx context.Context, msgs []*sarama.ProducerMessage) error {
	// Start span
	ctx, span := p.tracer.Start(ctx, "kafka send batch",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.MessagingSystemKey.String("kafka"),
			semconv.MessagingOperationNameKey.String("send"),
			semconv.MessagingBatchMessageCountKey.Int(len(msgs)),
		),
	)
	defer span.End()

	// Inject trace context into all messages
	propagator := otel.GetTextMapPropagator()
	for _, msg := range msgs {
		carrier := &producerMessageCarrier{msg: msg}
		propagator.Inject(ctx, carrier)
	}

	// Send messages
	err := p.producer.SendMessages(msgs)

	// Record result
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// Close closes the producer.
func (p *SyncProducer) Close() error {
	return p.producer.Close()
}

// ConsumerGroupHandler wraps a user's ConsumerGroupHandler with OpenTelemetry tracing and metrics.
type ConsumerGroupHandler struct {
	handler          sarama.ConsumerGroupHandler
	tracer           trace.Tracer
	meter            metric.Meter
	messagesReceived metric.Int64Counter
	receiveErrors    metric.Int64Counter
	processDuration  metric.Float64Histogram
}

// WrapConsumerGroupHandler wraps a Sarama ConsumerGroupHandler with OpenTelemetry tracing and metrics.
//
// Example:
//
//	handler := &MyConsumerHandler{}
//	wrappedHandler := kafka.WrapConsumerGroupHandler(handler)
//
//	consumer, _ := sarama.NewConsumerGroup(brokers, groupID, config)
//	consumer.Consume(ctx, topics, wrappedHandler)
func WrapConsumerGroupHandler(handler sarama.ConsumerGroupHandler) *ConsumerGroupHandler {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent: %v (consumer will not be instrumented)", err)
		}
	}

	// Create meter and metrics
	meter := otel.Meter(instrumentationName)

	messagesReceived, err := meter.Int64Counter(
		"messaging.kafka.messages.received",
		metric.WithDescription("Number of messages successfully received from Kafka"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create messages_received counter: %v", err)
	}

	receiveErrors, err := meter.Int64Counter(
		"messaging.kafka.receive.errors",
		metric.WithDescription("Number of message receive errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create receive_errors counter: %v", err)
	}

	processDuration, err := meter.Float64Histogram(
		"messaging.kafka.process.duration",
		metric.WithDescription("Duration of Kafka message processing"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		log.Printf("[Last9 Agent] Warning: Failed to create process_duration histogram: %v", err)
	}

	return &ConsumerGroupHandler{
		handler:          handler,
		tracer:           otel.Tracer(instrumentationName),
		meter:            meter,
		messagesReceived: messagesReceived,
		receiveErrors:    receiveErrors,
		processDuration:  processDuration,
	}
}

// Setup is called at the beginning of a new session, before ConsumeClaim.
func (h *ConsumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	return h.handler.Setup(session)
}

// Cleanup is called at the end of a session, once all ConsumeClaim goroutines have exited.
func (h *ConsumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	return h.handler.Cleanup(session)
}

// ConsumeClaim processes messages from a topic partition with automatic tracing and metrics.
func (h *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		startTime := time.Now()

		// Extract trace context from message headers
		ctx := ExtractContext(session.Context(), msg)

		// Start span for message processing
		ctx, span := h.tracer.Start(ctx, fmt.Sprintf("%s receive", msg.Topic),
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				semconv.MessagingSystemKey.String("kafka"),
				semconv.MessagingDestinationNameKey.String(msg.Topic),
				semconv.MessagingOperationNameKey.String("receive"),
				attribute.Int("messaging.kafka.partition", int(msg.Partition)),
				attribute.Int64("messaging.kafka.offset", msg.Offset),
			),
		)

		metricAttrs := []attribute.KeyValue{
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", msg.Topic),
		}

		// Create a wrapped session that includes the span context
		wrappedSession := &tracedSession{
			ConsumerGroupSession: session,
			ctx:                  ctx,
		}

		// Call the wrapped handler
		err := h.handler.ConsumeClaim(wrappedSession, claim)

		// Record processing duration
		duration := time.Since(startTime).Milliseconds()
		if h.processDuration != nil {
			h.processDuration.Record(ctx, float64(duration), metric.WithAttributes(metricAttrs...))
		}

		if err != nil {
			// Record error
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			if h.receiveErrors != nil {
				h.receiveErrors.Add(ctx, 1, metric.WithAttributes(metricAttrs...))
			}
			span.End()
			return err
		}

		// Record successful message processing
		if h.messagesReceived != nil {
			h.messagesReceived.Add(ctx, 1, metric.WithAttributes(metricAttrs...))
		}

		span.End()
	}
	return nil
}

// tracedSession wraps ConsumerGroupSession to provide trace context.
type tracedSession struct {
	sarama.ConsumerGroupSession
	ctx context.Context
}

// Context returns the trace-enriched context.
func (s *tracedSession) Context() context.Context {
	return s.ctx
}

// NewConsumerGroup creates a new Kafka consumer group.
// Use WrapConsumerGroupHandler to add tracing to your handler.
//
// Example:
//
//	consumer, err := kafka.NewConsumerGroup(kafka.ConsumerConfig{
//	    Brokers: []string{"localhost:9092"},
//	    GroupID: "my-consumer-group",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer consumer.Close()
//
//	handler := kafka.WrapConsumerGroupHandler(&MyHandler{})
//	consumer.Consume(ctx, []string{"my-topic"}, handler)
func NewConsumerGroup(cfg ConsumerConfig) (sarama.ConsumerGroup, error) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("failed to initialize Last9 agent: %w", err)
		}
	}

	// Use default config if not provided
	config := cfg.Config
	if config == nil {
		config = sarama.NewConfig()
		config.Version = sarama.V2_6_0_0
		config.Consumer.Return.Errors = true
	}

	// Create consumer group
	return sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, config)
}

// PropagateContext injects the current trace context into a Kafka message.
// This is automatically done by SendMessage, but can be used for manual control.
//
// Example:
//
//	msg := &sarama.ProducerMessage{
//	    Topic: "my-topic",
//	    Value: sarama.StringEncoder("Hello"),
//	}
//	kafka.PropagateContext(ctx, msg)
func PropagateContext(ctx context.Context, msg *sarama.ProducerMessage) {
	propagator := otel.GetTextMapPropagator()
	carrier := &producerMessageCarrier{msg: msg}
	propagator.Inject(ctx, carrier)
}

// ExtractContext extracts trace context from a Kafka message.
// This is automatically done by ConsumerGroupHandler, but can be used for manual control.
//
// Example:
//
//	for msg := range messages {
//	    ctx := kafka.ExtractContext(context.Background(), msg)
//	    processMessage(ctx, msg)
//	}
func ExtractContext(ctx context.Context, msg *sarama.ConsumerMessage) context.Context {
	propagator := otel.GetTextMapPropagator()
	carrier := &consumerMessageCarrier{msg: msg}
	return propagator.Extract(ctx, carrier)
}

// producerMessageCarrier adapts ProducerMessage to be a TextMapCarrier.
type producerMessageCarrier struct {
	msg *sarama.ProducerMessage
}

// Get retrieves a header value.
func (c *producerMessageCarrier) Get(key string) string {
	for _, h := range c.msg.Headers {
		if string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

// Set sets a header value.
func (c *producerMessageCarrier) Set(key, value string) {
	// Remove existing header with same key
	headers := make([]sarama.RecordHeader, 0, len(c.msg.Headers)+1)
	for _, h := range c.msg.Headers {
		if string(h.Key) != key {
			headers = append(headers, h)
		}
	}
	// Add new header
	headers = append(headers, sarama.RecordHeader{
		Key:   []byte(key),
		Value: []byte(value),
	})
	c.msg.Headers = headers
}

// Keys returns all header keys.
func (c *producerMessageCarrier) Keys() []string {
	keys := make([]string, len(c.msg.Headers))
	for i, h := range c.msg.Headers {
		keys[i] = string(h.Key)
	}
	return keys
}

// consumerMessageCarrier adapts ConsumerMessage to be a TextMapCarrier.
type consumerMessageCarrier struct {
	msg *sarama.ConsumerMessage
}

// Get retrieves a header value.
func (c *consumerMessageCarrier) Get(key string) string {
	for _, h := range c.msg.Headers {
		if h != nil && string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

// Set is not used for extraction, but required by interface.
func (c *consumerMessageCarrier) Set(key, value string) {}

// Keys returns all header keys.
func (c *consumerMessageCarrier) Keys() []string {
	keys := make([]string, 0, len(c.msg.Headers))
	for _, h := range c.msg.Headers {
		if h != nil {
			keys = append(keys, string(h.Key))
		}
	}
	return keys
}
