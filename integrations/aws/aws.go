// Package aws provides Last9 instrumentation for the AWS SDK v2.
package aws

import (
	"log"

	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	agent "github.com/last9/go-agent"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

// Option customises the AWS SDK instrumentation. It is an alias of
// otelaws.Option so callers do not need to import otelaws directly.
type Option = otelaws.Option

// InstrumentSDK adds OpenTelemetry tracing to an AWS SDK v2 configuration.
// Call it once after loading your AWS config, before creating any service clients:
//
//	cfg, err := awsconfig.LoadDefaultConfig(ctx)
//	if err != nil { ... }
//	aws.InstrumentSDK(&cfg)
//
//	s3Client  := s3.NewFromConfig(cfg)
//	sqsClient := sqs.NewFromConfig(cfg)
//
// Every SDK call will produce a span with attributes for the AWS service,
// operation, region, and request ID. Per-service attributes (e.g. DynamoDB
// table name, SQS queue URL, SNS topic ARN) are captured automatically.
//
// To override or extend which attributes are recorded, pass Option values:
//
//	aws.InstrumentSDK(&cfg,
//	    otelaws.WithAttributeBuilder(myCustomAttributeBuilder),
//	)
func InstrumentSDK(cfg *awsconfig.Config, opts ...Option) {
	if !agent.IsInitialized() {
		if err := agent.Start(); err != nil {
			log.Printf("[Last9 Agent] Warning: Failed to auto-start agent for AWS SDK instrumentation: %v", err)
		}
	}
	otelaws.AppendMiddlewares(&cfg.APIOptions, opts...)
}
