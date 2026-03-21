package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

// newTestDetector creates an EC2Detector pointed at the given test server URL.
func newTestDetector(serverURL string) *EC2Detector {
	d := NewEC2Detector()
	d.endpoint = serverURL
	return d
}

// fakeIMDS returns an http.Handler simulating the EC2 IMDS endpoint.
func fakeIMDS(metadata map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token endpoint
		if r.URL.Path == "/latest/api/token" && r.Method == http.MethodPut {
			if r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				http.Error(w, "missing TTL header", http.StatusBadRequest)
				return
			}
			w.Write([]byte("test-imds-token"))
			return
		}

		// Verify token on metadata requests
		if r.Header.Get("X-aws-ec2-metadata-token") != "test-imds-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Serve metadata
		if val, ok := metadata[r.URL.Path]; ok {
			w.Write([]byte(val))
			return
		}
		http.NotFound(w, r)
	})
}

func TestDetect_FullMetadataAndTags(t *testing.T) {
	srv := httptest.NewServer(fakeIMDS(map[string]string{
		"/latest/meta-data/instance-id":              "i-0abc123def456",
		"/latest/meta-data/placement/availability-zone": "us-east-1a",
		"/latest/meta-data/instance-type":             "m5.xlarge",
		"/latest/meta-data/hostname":                  "ip-10-0-1-42.ec2.internal",
		"/latest/meta-data/tags/instance":             "Name\napp\nteam",
		"/latest/meta-data/tags/instance/Name":        "my-service-prod",
		"/latest/meta-data/tags/instance/app":         "my-service",
		"/latest/meta-data/tags/instance/team":        "platform",
	}))
	defer srv.Close()

	d := newTestDetector(srv.URL)
	res, err := d.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	attrs := res.Attributes()
	attrMap := make(map[attribute.Key]string, len(attrs))
	for _, a := range attrs {
		attrMap[a.Key] = a.Value.AsString()
	}

	expected := map[attribute.Key]string{
		semconv.CloudProviderKey:        "aws",
		semconv.CloudRegionKey:          "us-east-1",
		semconv.CloudAvailabilityZoneKey: "us-east-1a",
		semconv.HostIDKey:               "i-0abc123def456",
		semconv.HostTypeKey:             "m5.xlarge",
		semconv.HostNameKey:             "ip-10-0-1-42.ec2.internal",
		"ec2.tag.Name":                  "my-service-prod",
		"ec2.tag.app":                   "my-service",
		"ec2.tag.team":                  "platform",
	}

	for key, want := range expected {
		got, ok := attrMap[key]
		if !ok {
			t.Errorf("missing attribute %s", key)
			continue
		}
		if got != want {
			t.Errorf("attribute %s = %q, want %q", key, got, want)
		}
	}
}

func TestDetect_NoTags(t *testing.T) {
	srv := httptest.NewServer(fakeIMDS(map[string]string{
		"/latest/meta-data/instance-id":              "i-0abc123def456",
		"/latest/meta-data/placement/availability-zone": "eu-west-1b",
		"/latest/meta-data/instance-type":             "t3.micro",
		"/latest/meta-data/hostname":                  "ip-10-0-1-1.ec2.internal",
		// No tags/instance endpoint — simulates tags not enabled
	}))
	defer srv.Close()

	d := newTestDetector(srv.URL)
	res, err := d.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	attrs := res.Attributes()
	for _, a := range attrs {
		if key := string(a.Key); len(key) > 8 && key[:8] == "ec2.tag." {
			t.Errorf("unexpected tag attribute: %s", a.Key)
		}
	}

	// Should still have core metadata
	attrMap := make(map[attribute.Key]string, len(attrs))
	for _, a := range attrs {
		attrMap[a.Key] = a.Value.AsString()
	}
	if attrMap[semconv.CloudRegionKey] != "eu-west-1" {
		t.Errorf("region = %q, want eu-west-1", attrMap[semconv.CloudRegionKey])
	}
}

func TestDetect_IMDSUnreachable(t *testing.T) {
	// Point at a server that immediately closes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ec2", http.StatusForbidden)
	}))
	defer srv.Close()

	d := newTestDetector(srv.URL)
	res, err := d.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect should not error on non-EC2: %v", err)
	}

	if len(res.Attributes()) != 0 {
		t.Errorf("expected empty resource on non-EC2, got %d attributes", len(res.Attributes()))
	}
}

func TestDetect_PartialMetadata(t *testing.T) {
	// Only instance-id available, everything else 404
	srv := httptest.NewServer(fakeIMDS(map[string]string{
		"/latest/meta-data/instance-id": "i-partial",
	}))
	defer srv.Close()

	d := newTestDetector(srv.URL)
	res, err := d.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	attrs := res.Attributes()
	attrMap := make(map[attribute.Key]string, len(attrs))
	for _, a := range attrs {
		attrMap[a.Key] = a.Value.AsString()
	}

	if attrMap[semconv.HostIDKey] != "i-partial" {
		t.Errorf("host.id = %q, want i-partial", attrMap[semconv.HostIDKey])
	}
	// No AZ means no region
	if _, ok := attrMap[semconv.CloudRegionKey]; ok {
		t.Error("region should not be set when AZ is missing")
	}
}

func TestDetect_TokenValidation(t *testing.T) {
	// Verify detector sends the token on metadata requests
	tokenSeen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest/api/token" {
			w.Write([]byte("secret-token-123"))
			return
		}
		if r.Header.Get("X-aws-ec2-metadata-token") == "secret-token-123" {
			tokenSeen = true
			w.Write([]byte("i-tokentest"))
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	d := newTestDetector(srv.URL)
	d.Detect(context.Background())

	if !tokenSeen {
		t.Error("detector did not send IMDSv2 token on metadata request")
	}
}
