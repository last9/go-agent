//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"go.opentelemetry.io/otel/trace"

	"github.com/last9/go-agent"
	kafkaagent "github.com/last9/go-agent/integrations/kafka"
	"github.com/last9/go-agent/tests/testutil"
)

// setupKafkaTest sets up a Kafka container and mock collector for testing.
func setupKafkaTest(t *testing.T) (*kafka.KafkaContainer, *testutil.MockCollector, context.Context, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Start Kafka container
	kafkaContainer, err := kafka.Run(ctx, "confluentinc/confluent-local:7.6.0")
	require.NoError(t, err, "failed to start Kafka container")

	t.Cleanup(func() {
		// Use a fresh context for cleanup to avoid cancellation issues
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := kafkaContainer.Terminate(cleanupCtx); err != nil {
			t.Logf("Warning: failed to terminate Kafka container: %v", err)
		}
		cancel()
	})

	// Initialize agent
	agent.Start()
	t.Cleanup(func() {
		agent.Shutdown()
	})

	// Create mock collector
	collector := testutil.NewMockCollector()
	t.Cleanup(func() {
		collector.Shutdown(context.Background())
	})

	return kafkaContainer, collector, ctx, cancel
}

func TestKafkaSyncProducer_SendMessage(t *testing.T) {
	kafkaContainer, collector, ctx, cancel := setupKafkaTest(t)
	defer cancel()

	// Get Kafka broker address
	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)

	// Create producer
	producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
		Brokers: brokers,
	})
	require.NoError(t, err)
	defer producer.Close()

	// Create a parent span to test context propagation
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-parent")
	defer parentSpan.End()

	// Send message
	topic := "test-topic-send-message"
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.StringEncoder("Hello Kafka"),
	}

	partition, offset, err := producer.SendMessage(ctx, msg)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, partition, int32(0))
	assert.GreaterOrEqual(t, offset, int64(0))

	// Wait for spans to be recorded
	time.Sleep(100 * time.Millisecond)

	// Verify spans
	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 2) // parent + producer

	// Find producer span
	producerSpan := testutil.FindSpanByName(spans, fmt.Sprintf("%s send", topic))
	require.NotNil(t, producerSpan, "producer span not found")

	// Verify producer span attributes
	testutil.AssertSpanKind(t, producerSpan, trace.SpanKindProducer)
	testutil.AssertSpanAttribute(t, producerSpan, "messaging.system", "kafka")
	testutil.AssertSpanAttribute(t, producerSpan, "messaging.destination.name", topic)
	testutil.AssertSpanAttribute(t, producerSpan, "messaging.operation.name", "send")
	testutil.AssertSpanAttributeInt(t, producerSpan, "messaging.kafka.partition", int64(partition))
	testutil.AssertSpanAttributeInt(t, producerSpan, "messaging.kafka.offset", offset)

	// Verify no errors
	testutil.AssertSpanNoError(t, producerSpan)
}

func TestKafkaSyncProducer_SendMessages_Batch(t *testing.T) {
	kafkaContainer, collector, ctx, cancel := setupKafkaTest(t)
	defer cancel()

	// Get Kafka broker address
	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)

	// Create producer
	producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
		Brokers: brokers,
	})
	require.NoError(t, err)
	defer producer.Close()

	// Create a parent span
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "test-batch-parent")
	defer parentSpan.End()

	// Send batch messages
	topic := "test-topic-batch"
	messages := []*sarama.ProducerMessage{
		{Topic: topic, Value: sarama.StringEncoder("Message 1")},
		{Topic: topic, Value: sarama.StringEncoder("Message 2")},
		{Topic: topic, Value: sarama.StringEncoder("Message 3")},
	}

	err = producer.SendMessages(ctx, messages)
	require.NoError(t, err)

	// Wait for spans
	time.Sleep(100 * time.Millisecond)

	// Verify spans
	spans := collector.GetSpans()
	testutil.AssertSpanCount(t, spans, 2) // parent + batch span

	// Find batch span
	batchSpan := testutil.FindSpanByName(spans, "kafka send batch")
	require.NotNil(t, batchSpan, "batch span not found")

	// Verify batch span attributes
	testutil.AssertSpanKind(t, batchSpan, trace.SpanKindProducer)
	testutil.AssertSpanAttribute(t, batchSpan, "messaging.system", "kafka")
	testutil.AssertSpanAttribute(t, batchSpan, "messaging.operation.name", "send")
	testutil.AssertSpanAttributeInt(t, batchSpan, "messaging.batch.message_count", int64(len(messages)))

	// Verify no errors
	testutil.AssertSpanNoError(t, batchSpan)
}

// TestMessageHandler is a test consumer handler
type TestMessageHandler struct {
	messagesReceived chan *sarama.ConsumerMessage
	ctx              context.Context
	ready            chan bool
}

func (h *TestMessageHandler) Setup(session sarama.ConsumerGroupSession) error {
	// Signal that consumer is ready
	if h.ready != nil {
		close(h.ready)
	}
	return nil
}

func (h *TestMessageHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	return nil
}

func (h *TestMessageHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		// Store context from session (should contain trace info)
		h.ctx = session.Context()

		// Send message to channel
		select {
		case h.messagesReceived <- msg:
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timeout sending message to channel")
		}

		// Mark message as processed
		session.MarkMessage(msg, "")
	}
	return nil
}

func TestKafka_EndToEnd_ContextPropagation(t *testing.T) {
	// This test validates end-to-end trace context propagation from producer to consumer.
	// The consumer wrapper uses a tracedClaim that intercepts messages and creates spans
	// with the trace context extracted from message headers.

	kafkaContainer, collector, ctx, cancel := setupKafkaTest(t)
	defer cancel()

	// Get Kafka broker address
	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)

	topic := "test-topic-e2e"

	// Create producer
	producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
		Brokers: brokers,
	})
	require.NoError(t, err)
	defer producer.Close()

	// Create consumer
	consumer, err := kafkaagent.NewConsumerGroup(kafkaagent.ConsumerConfig{
		Brokers: brokers,
		GroupID: "test-consumer-group-e2e",
	})
	require.NoError(t, err)
	defer consumer.Close()

	// Create handler with message channel and ready signal
	handler := &TestMessageHandler{
		messagesReceived: make(chan *sarama.ConsumerMessage, 1),
		ready:            make(chan bool),
	}
	wrappedHandler := kafkaagent.WrapConsumerGroupHandler(handler)

	// Start consumer in background
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()

	consumerErrors := make(chan error, 1)
	go func() {
		err := consumer.Consume(consumerCtx, []string{topic}, wrappedHandler)
		if err != nil && !errors.Is(err, context.Canceled) {
			consumerErrors <- err
		}
	}()

	// Wait for consumer to be ready (with timeout)
	select {
	case <-handler.ready:
		t.Log("Consumer is ready")
	case <-time.After(30 * time.Second):
		t.Fatal("Timeout waiting for consumer to be ready")
	}

	// Additional wait for consumer group stabilization
	time.Sleep(2 * time.Second)

	// Send message with trace context
	ctx, producerSpan := testutil.CreateTestSpan(ctx, "e2e-test-producer")
	producerTraceID := testutil.GetTraceIDFromContext(ctx)

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.StringEncoder("End-to-end test message"),
	}

	partition, offset, err := producer.SendMessage(ctx, msg)
	require.NoError(t, err)
	t.Logf("Sent message to partition %d at offset %d", partition, offset)

	producerSpan.End()

	// Wait for consumer to receive message (with timeout)
	var receivedMsg *sarama.ConsumerMessage
	select {
	case receivedMsg = <-handler.messagesReceived:
		t.Logf("Received message: %s", string(receivedMsg.Value))
	case err := <-consumerErrors:
		t.Fatalf("Consumer error: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("Timeout waiting for consumer to receive message")
	}

	// Stop consumer
	consumerCancel()

	// Wait for spans to be recorded
	time.Sleep(500 * time.Millisecond)

	// Verify spans
	spans := collector.GetSpans()
	t.Logf("Captured %d spans", len(spans))
	for i, span := range spans {
		t.Logf("Span %d: %s (kind: %v, trace: %s)", i, span.Name(), span.SpanKind(), span.SpanContext().TraceID())
	}

	// Find producer and consumer spans
	producerSpan2 := testutil.FindSpanByName(spans, fmt.Sprintf("%s send", topic))
	require.NotNil(t, producerSpan2, "producer span not found")

	consumerSpan := testutil.FindSpanByName(spans, fmt.Sprintf("%s receive", topic))
	require.NotNil(t, consumerSpan, "consumer span not found")

	// CRITICAL: Verify context propagation - consumer should be in the same trace as producer
	assert.Equal(t, producerTraceID, consumerSpan.SpanContext().TraceID(),
		"consumer span should have same trace ID as producer")

	// Verify producer span
	testutil.AssertSpanKind(t, producerSpan2, trace.SpanKindProducer)
	testutil.AssertSpanAttribute(t, producerSpan2, "messaging.system", "kafka")
	testutil.AssertSpanAttribute(t, producerSpan2, "messaging.destination.name", topic)

	// Verify consumer span
	testutil.AssertSpanKind(t, consumerSpan, trace.SpanKindConsumer)
	testutil.AssertSpanAttribute(t, consumerSpan, "messaging.system", "kafka")
	testutil.AssertSpanAttribute(t, consumerSpan, "messaging.destination.name", topic)
	testutil.AssertSpanAttributeInt(t, consumerSpan, "messaging.kafka.partition", int64(receivedMsg.Partition))
	testutil.AssertSpanAttributeInt(t, consumerSpan, "messaging.kafka.offset", receivedMsg.Offset)

	// Verify consumer context had trace info
	assert.True(t, testutil.HasTraceContext(handler.ctx), "consumer context should have trace info")
	consumerTraceID := testutil.GetTraceIDFromContext(handler.ctx)
	assert.Equal(t, producerTraceID, consumerTraceID, "consumer context should have producer's trace ID")
}

func TestKafkaSyncProducer_SendMessage_Error(t *testing.T) {
	collector := testutil.NewMockCollector()
	defer collector.Shutdown(context.Background())

	agent.Start()
	defer agent.Shutdown()

	// Create producer with invalid broker
	// Note: Producer creation might fail immediately with newer Kafka client versions
	producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
		Brokers: []string{"invalid-broker:9092"},
	})
	if err != nil {
		t.Logf("Producer creation failed (expected): %v", err)
		t.Skip("Producer creation with invalid broker failed immediately - behavior varies by client version")
		return
	}
	defer producer.Close()

	ctx := context.Background()
	ctx, parentSpan := testutil.CreateTestSpan(ctx, "error-test-parent")
	defer parentSpan.End()

	// Try to send message (should fail)
	msg := &sarama.ProducerMessage{
		Topic: "test-topic-error",
		Value: sarama.StringEncoder("This should fail"),
	}

	_, _, err = producer.SendMessage(ctx, msg)
	// Verify we got an error (expected behavior with invalid broker)
	if err != nil {
		t.Logf("Got expected error during send: %v", err)
	} else {
		t.Error("Expected error with invalid broker but got none")
	}

	// Wait for spans
	time.Sleep(100 * time.Millisecond)

	// Verify spans were captured
	spans := collector.GetSpans()
	assert.GreaterOrEqual(t, len(spans), 1, "should have at least 1 span (parent span)")

	// If error occurred and span was created, verify it
	producerSpan := testutil.FindSpanByName(spans, "test-topic-error send")
	if producerSpan != nil {
		t.Log("Producer span found, verifying error was recorded")
		testutil.AssertSpanError(t, producerSpan)
	} else {
		t.Log("Producer span not found (error may have occurred before span creation)")
	}
}

func TestKafkaConsumer_WrapHandler(t *testing.T) {
	// Test that WrapConsumerGroupHandler properly wraps the handler
	handler := &TestMessageHandler{
		messagesReceived: make(chan *sarama.ConsumerMessage, 1),
	}

	wrappedHandler := kafkaagent.WrapConsumerGroupHandler(handler)
	require.NotNil(t, wrappedHandler)

	// Verify it's a ConsumerGroupHandler
	var _ sarama.ConsumerGroupHandler = wrappedHandler
}
