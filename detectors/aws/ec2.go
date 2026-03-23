// Package aws provides cloud resource detectors for AWS environments.
package aws

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

const (
	imdsBase     = "http://169.254.169.254"
	imdsTokenTTL = "300"
	imdsTimeout  = 2 * time.Second
)

// EC2Detector detects EC2 instance metadata and tags using IMDSv2.
// It implements the resource.Detector interface.
type EC2Detector struct {
	client   *http.Client
	endpoint string
}

// NewEC2Detector creates a detector that fetches EC2 instance metadata
// and instance tags via IMDSv2.
func NewEC2Detector() *EC2Detector {
	return &EC2Detector{
		endpoint: imdsBase,
		client: &http.Client{
			Timeout: imdsTimeout,
		},
	}
}

// Detect fetches EC2 metadata and tags, returning them as resource attributes.
// Returns an empty resource (not an error) when not running on EC2 or if
// IMDS is unreachable, so it never blocks application startup.
func (d *EC2Detector) Detect(ctx context.Context) (*resource.Resource, error) {
	token, err := d.getToken(ctx)
	if err != nil {
		return resource.Empty(), nil
	}

	attrs := []attribute.KeyValue{
		semconv.CloudProviderAWS,
	}

	instanceID := d.getMetadata(ctx, token, "/latest/meta-data/instance-id")
	az := d.getMetadata(ctx, token, "/latest/meta-data/placement/availability-zone")
	instanceType := d.getMetadata(ctx, token, "/latest/meta-data/instance-type")
	hostname := d.getMetadata(ctx, token, "/latest/meta-data/hostname")

	if az != "" {
		attrs = append(attrs, semconv.CloudAvailabilityZone(az))
		// Region = AZ minus the trailing letter (us-east-1a -> us-east-1)
		region := az[:len(az)-1]
		attrs = append(attrs, semconv.CloudRegion(region))
	}
	if instanceID != "" {
		attrs = append(attrs, semconv.HostID(instanceID))
	}
	if instanceType != "" {
		attrs = append(attrs, semconv.HostType(instanceType))
	}
	if hostname != "" {
		attrs = append(attrs, semconv.HostName(hostname))
	}

	// Fetch instance tags (requires "Allow tags in instance metadata" on the instance)
	tagAttrs := d.detectTags(ctx, token)
	attrs = append(attrs, tagAttrs...)

	return resource.NewSchemaless(attrs...), nil
}

// detectTags fetches EC2 instance tags from the IMDS tags endpoint.
// Returns empty slice if tags are not available (feature not enabled or no tags).
func (d *EC2Detector) detectTags(ctx context.Context, token string) []attribute.KeyValue {
	tagKeysRaw := d.getMetadata(ctx, token, "/latest/meta-data/tags/instance")
	if tagKeysRaw == "" {
		return nil
	}

	var attrs []attribute.KeyValue
	for _, key := range strings.Split(tagKeysRaw, "\n") {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value := d.getMetadata(ctx, token, "/latest/meta-data/tags/instance/"+key)
		if value != "" {
			attrs = append(attrs, attribute.String("ec2.tag."+key, value))
		}
	}
	return attrs
}

// getToken requests an IMDSv2 session token.
func (d *EC2Detector) getToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, d.endpoint+"/latest/api/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", imdsTokenTTL)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("IMDSv2 token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDSv2 token request returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// getMetadata fetches a single IMDS metadata value. Returns empty string on failure.
func (d *EC2Detector) getMetadata(ctx context.Context, token, path string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.endpoint+path, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-aws-ec2-metadata-token", token)

	resp, err := d.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}
