package aws_test

import (
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	last9aws "github.com/last9/go-agent/integrations/aws"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

func TestInstrumentSDK_AppendsMiddleware(t *testing.T) {
	cfg := awsconfig.Config{}
	before := len(cfg.APIOptions)

	last9aws.InstrumentSDK(&cfg)

	if len(cfg.APIOptions) <= before {
		t.Error("InstrumentSDK should append at least one middleware to cfg.APIOptions")
	}
}

func TestInstrumentSDK_NilSafe(t *testing.T) {
	// Should not panic when called with a zero-value config.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InstrumentSDK panicked: %v", r)
		}
	}()
	cfg := awsconfig.Config{}
	last9aws.InstrumentSDK(&cfg)
}

func TestInstrumentSDK_AcceptsOptions(t *testing.T) {
	cfg := awsconfig.Config{}
	before := len(cfg.APIOptions)

	// Passing explicit options should still work without panic.
	last9aws.InstrumentSDK(&cfg, otelaws.WithTracerProvider(nil))

	if len(cfg.APIOptions) <= before {
		t.Error("InstrumentSDK with options should still append middleware")
	}
}
