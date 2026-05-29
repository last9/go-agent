package grpcgateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDefaultExcluded_ExactPaths(t *testing.T) {
	for _, path := range defaultExcludedPaths {
		assert.True(t, isDefaultExcluded(path), "path %q should be excluded", path)
	}
}

func TestIsDefaultExcluded_Prefixes(t *testing.T) {
	cases := []string{
		"/actuator/health",
		"/actuator/info",
		"/actuator/prometheus",
		"/eureka/apps/MY-SERVICE",
		"/eureka/apps/GO-AUTH/localhost:8080:8080",
	}
	for _, path := range cases {
		assert.True(t, isDefaultExcluded(path), "path %q should be excluded", path)
	}
}

func TestIsDefaultExcluded_AllowedPaths(t *testing.T) {
	cases := []string{
		"/v1/users",
		"/api/orders",
		"/actuator",       // prefix requires trailing slash — bare path is allowed
		"/eureka/status",  // not under /eureka/apps/
		"/healthcheck",    // not an exact match for /health
		"/metrics/custom", // subdirectory of /metrics, not exact
		"/",
	}
	for _, path := range cases {
		assert.False(t, isDefaultExcluded(path), "path %q should NOT be excluded", path)
	}
}
