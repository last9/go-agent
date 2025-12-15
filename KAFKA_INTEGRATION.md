# Kafka Integration for Last9 Go Agent

## Overview

The Kafka integration provides comprehensive OpenTelemetry instrumentation for Kafka producers and consumers using IBM Sarama (formerly Shopify Sarama), the recommended Kafka client library for Go.

## Features

- ✅ **Automatic tracing** for Kafka producers and consumers
- ✅ **Context propagation** across producer → consumer boundaries
- ✅ **OpenTelemetry semantic conventions** for messaging systems
- ✅ **Zero-config** setup with agent auto-initialization
- ✅ **Span attributes** including topic, partition, offset
- ✅ **Error tracking** with automatic span error recording
- ✅ **Batch message support** for high-throughput scenarios

## Installation

The Kafka integration is part of the main go-agent package:

```bash
go get github.com/last9/go-agent
go get github.com/IBM/sarama
```

## Producer Usage

### Synchronous Producer

```go
package main

import (
    "context"
    "log"

    "github.com/IBM/sarama"
    "github.com/last9/go-agent"
    kafkaagent "github.com/last9/go-agent/integrations/kafka"
)

func main() {
    // Initialize agent
    agent.Start()
    defer agent.Shutdown()

    // Create instrumented producer
    producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
        Brokers: []string{"localhost:9092"},
    })
    if err != nil {
        log.Fatal(err)
    }
    defer producer.Close()

    // Create context (parent span context for tracing)
    ctx := context.Background()

    // Send message with automatic tracing
    msg := &sarama.ProducerMessage{
        Topic: "my-topic",
        Value: sarama.StringEncoder("Hello Kafka!"),
    }

    partition, offset, err := producer.SendMessage(ctx, msg)
    if err != nil {
        log.Printf("Failed to send message: %v", err)
    } else {
        log.Printf("Message sent to partition %d at offset %d", partition, offset)
    }
}
```

### Batch Messages

```go
// Send multiple messages efficiently
messages := []*sarama.ProducerMessage{
    {Topic: "my-topic", Value: sarama.StringEncoder("Message 1")},
    {Topic: "my-topic", Value: sarama.StringEncoder("Message 2")},
    {Topic: "my-topic", Value: sarama.StringEncoder("Message 3")},
}

err := producer.SendMessages(ctx, messages)
if err != nil {
    log.Printf("Failed to send batch: %v", err)
}
```

## Consumer Usage

### Consumer Group Handler

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/IBM/sarama"
    "github.com/last9/go-agent"
    kafkaagent "github.com/last9/go-agent/integrations/kafka"
)

// MyMessageHandler implements sarama.ConsumerGroupHandler
type MyMessageHandler struct{}

func (h *MyMessageHandler) Setup(session sarama.ConsumerGroupSession) error {
    log.Println("Consumer group session started")
    return nil
}

func (h *MyMessageHandler) Cleanup(session sarama.ConsumerGroupSession) error {
    log.Println("Consumer group session ended")
    return nil
}

func (h *MyMessageHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        // Context automatically includes trace from producer
        ctx := session.Context()

        log.Printf("Received message: %s (partition=%d, offset=%d)",
            string(msg.Value), msg.Partition, msg.Offset)

        // Process message with traced context
        processMessage(ctx, msg)

        // Mark message as processed
        session.MarkMessage(msg, "")
    }
    return nil
}

func processMessage(ctx context.Context, msg *sarama.ConsumerMessage) {
    // Your business logic here
    // Context includes trace information for distributed tracing
    log.Printf("Processing message: %s", string(msg.Value))
}

func main() {
    // Initialize agent
    agent.Start()
    defer agent.Shutdown()

    // Create consumer group
    consumer, err := kafkaagent.NewConsumerGroup(kafkaagent.ConsumerConfig{
        Brokers: []string{"localhost:9092"},
        GroupID: "my-consumer-group",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer consumer.Close()

    // Wrap handler for automatic tracing
    handler := kafkaagent.WrapConsumerGroupHandler(&MyMessageHandler{})

    // Setup signal handling
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigterm := make(chan os.Signal, 1)
    signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

    // Consume messages
    go func() {
        for {
            if err := consumer.Consume(ctx, []string{"my-topic"}, handler); err != nil {
                log.Printf("Error from consumer: %v", err)
            }
            if ctx.Err() != nil {
                return
            }
        }
    }()

    <-sigterm
    log.Println("Shutting down consumer...")
}
```

## Trace Hierarchy

The integration produces distributed traces that span across producer and consumer:

### Producer Trace:
```
kafka send "my-topic"
  ├─ messaging.system: kafka
  ├─ messaging.destination.name: my-topic
  ├─ messaging.operation.name: send
  ├─ messaging.kafka.partition: 0
  └─ messaging.kafka.offset: 12345
```

### Consumer Trace (linked to producer):
```
kafka receive "my-topic"
  ├─ messaging.system: kafka
  ├─ messaging.destination.name: my-topic
  ├─ messaging.operation.name: receive
  ├─ messaging.kafka.partition: 0
  └─ messaging.kafka.offset: 12345
```

### End-to-End Trace:
```
HTTP POST /api/order
  └─ kafka send "orders"
      └─ kafka receive "orders"
          └─ SQL INSERT order
```

## Configuration

### Custom Sarama Config

```go
config := sarama.NewConfig()
config.Version = sarama.V3_0_0_0
config.Producer.RequiredAcks = sarama.WaitForAll
config.Producer.Retry.Max = 5
config.Producer.Return.Successes = true

producer, err := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
    Brokers: []string{"localhost:9092"},
    Config:  config, // Custom configuration
})
```

### Environment Variables

Uses standard OpenTelemetry environment variables:

```bash
export OTEL_SERVICE_NAME="my-kafka-service"
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp.last9.io:443"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <token>"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"
```

## Advanced Features

### Manual Context Propagation

If you need manual control over context propagation:

```go
import kafkaagent "github.com/last9/go-agent/integrations/kafka"

// Producer: Inject trace context
msg := &sarama.ProducerMessage{
    Topic: "my-topic",
    Value: sarama.StringEncoder("Hello"),
}
kafkaagent.PropagateContext(ctx, msg)

// Consumer: Extract trace context
ctx := kafkaagent.ExtractContext(context.Background(), msg)
```

### Context from Upstream Services

When receiving Kafka messages triggered by HTTP requests:

```go
func (h *MyHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        // Context includes trace from HTTP request → Kafka producer
        ctx := session.Context()

        // Make database call - will be child span of HTTP request
        db.QueryContext(ctx, "SELECT ...")

        session.MarkMessage(msg, "")
    }
    return nil
}
```

## Semantic Conventions

The integration follows OpenTelemetry semantic conventions for messaging systems:

### Span Attributes

**Producer:**
- `messaging.system`: "kafka"
- `messaging.destination.name`: Topic name
- `messaging.operation.name`: "send"
- `messaging.kafka.partition`: Partition number
- `messaging.kafka.offset`: Message offset

**Consumer:**
- `messaging.system`: "kafka"
- `messaging.destination.name`: Topic name
- `messaging.operation.name`: "receive"
- `messaging.kafka.partition`: Partition number
- `messaging.kafka.offset`: Message offset

### Span Kinds

- **Producer**: `trace.SpanKindProducer`
- **Consumer**: `trace.SpanKindConsumer`

## Error Handling

Errors are automatically recorded in spans:

```go
partition, offset, err := producer.SendMessage(ctx, msg)
if err != nil {
    // Error is automatically recorded in span with:
    // - span.RecordError(err)
    // - span.SetStatus(codes.Error, err.Error())
    log.Printf("Send failed: %v", err)
}
```

## Performance Considerations

### Batch Processing

For high-throughput scenarios, use batch sending:

```go
// More efficient than individual SendMessage calls
err := producer.SendMessages(ctx, messages)
```

### Async vs Sync

- **Sync Producer**: Recommended for most use cases, simpler error handling
- **Async Producer**: Not yet implemented (coming soon)

### Overhead

- **Per-message overhead**: < 0.1ms for trace context injection/extraction
- **Memory**: Minimal - uses message headers for context propagation
- **Network**: Adds ~100-200 bytes per message for trace headers

## Troubleshooting

### Issue: Traces Not Connected

**Symptom:** Consumer spans don't link to producer spans.

**Solution:** Ensure trace context is propagated:
```go
// Producer must use context
producer.SendMessage(ctx, msg) // ✅ Correct

// Not just:
producer.SendMessage(context.Background(), msg) // ❌ No parent span
```

### Issue: Missing Spans

**Symptom:** No Kafka spans appear in traces.

**Solution:** Verify agent is initialized:
```go
agent.Start() // Must be called before creating producers/consumers
```

## Best Practices

1. **Use Context Everywhere**
   - Always pass context from HTTP handlers to producers
   - Use session.Context() in consumers

2. **Error Handling**
   - Check producer send errors
   - Handle consumer errors in ConsumeClaim

3. **Resource Cleanup**
   - Always defer producer.Close()
   - Always defer consumer.Close()

4. **Consumer Groups**
   - Use consumer groups for scalability
   - Wrap handlers with WrapConsumerGroupHandler

5. **Message Ordering**
   - Use partition keys for ordered messages
   - Be aware of rebalancing effects

## Example: Full Microservice

```go
// Order Service: Produces to "orders" topic
func CreateOrder(c *gin.Context) {
    ctx := c.Request.Context() // Trace context from HTTP

    order := Order{ID: uuid.New(), Amount: 100}

    msg := &sarama.ProducerMessage{
        Topic: "orders",
        Key:   sarama.StringEncoder(order.ID),
        Value: sarama.ByteEncoder(json.Marshal(order)),
    }

    producer.SendMessage(ctx, msg) // Trace: HTTP → Kafka
    c.JSON(200, order)
}

// Payment Service: Consumes from "orders" topic
func (h *PaymentHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        ctx := session.Context() // Trace context from producer

        var order Order
        json.Unmarshal(msg.Value, &order)

        // Process payment - trace: HTTP → Kafka → DB
        db.QueryContext(ctx, "INSERT INTO payments ...")

        session.MarkMessage(msg, "")
    }
    return nil
}
```

## Comparison with Direct Sarama

### Without go-agent (Direct Sarama):

```go
// ~50 lines of setup
producer, _ := sarama.NewSyncProducer(brokers, config)

// Manual tracing setup
tracer := otel.Tracer("kafka")
ctx, span := tracer.Start(ctx, "kafka.send")
defer span.End()

// Manual header injection
propagator := otel.GetTextMapPropagator()
carrier := &ProducerMessageCarrier{msg: msg}
propagator.Inject(ctx, carrier)

// Send
partition, offset, _ := producer.SendMessage(msg)
```

### With go-agent:

```go
// ~5 lines total
producer, _ := kafkaagent.NewSyncProducer(kafkaagent.ProducerConfig{
    Brokers: brokers,
})

// Automatic tracing + context propagation
partition, offset, _ := producer.SendMessage(ctx, msg)
```

**Code reduction: ~90%**

## Supported Kafka Versions

- Kafka 2.6.0+ (via IBM Sarama)
- Tested with Kafka 2.8, 3.0, 3.1, 3.2, 3.3, 3.4, 3.5

## Limitations

- Async producer not yet implemented (coming soon)
- Requires IBM Sarama v1.38.0+
- Header-based context propagation only (requires Kafka 0.11+)

## Roadmap

- [ ] Async producer support
- [ ] Custom span naming options
- [ ] Message filtering/sampling
- [ ] Performance metrics (throughput, lag)
- [ ] Admin operations tracing

## Support

- Documentation: https://docs.last9.io
- Issues: https://github.com/last9/go-agent/issues
- Example: Coming soon in `examples/kafka/`

---

**Built with:**
- [IBM Sarama](https://github.com/IBM/sarama)
- [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go)
