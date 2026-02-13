// Package routematcher provides path matching for HTTP route exclusion.
// It supports exact paths, prefix matching, and glob patterns.
package routematcher

import (
	"path"
	"strings"
)

// RouteMatcher evaluates paths against three layers of rules (cheapest first):
// exact map lookup, prefix scan, then glob matching via path.Match.
type RouteMatcher struct {
	exact    map[string]struct{}
	prefixes []string
	patterns []string
}

// New creates a RouteMatcher from the given sets of rules.
// Any or all slices may be nil/empty.
func New(exactPaths, prefixes, patterns []string) *RouteMatcher {
	exact := make(map[string]struct{}, len(exactPaths))
	for _, p := range exactPaths {
		if p != "" {
			exact[p] = struct{}{}
		}
	}

	var cleanPrefixes []string
	for _, p := range prefixes {
		if p != "" {
			cleanPrefixes = append(cleanPrefixes, p)
		}
	}

	var cleanPatterns []string
	for _, p := range patterns {
		if p != "" {
			cleanPatterns = append(cleanPatterns, p)
		}
	}

	return &RouteMatcher{
		exact:    exact,
		prefixes: cleanPrefixes,
		patterns: cleanPatterns,
	}
}

// ShouldExclude returns true if the path matches any exclusion rule.
// Safe to call on a nil receiver (returns false).
func (rm *RouteMatcher) ShouldExclude(urlPath string) bool {
	if rm == nil {
		return false
	}

	// Layer 1: exact match (O(1))
	if _, ok := rm.exact[urlPath]; ok {
		return true
	}

	// Layer 2: prefix match
	for _, prefix := range rm.prefixes {
		if strings.HasPrefix(urlPath, prefix) {
			return true
		}
	}

	// Layer 3: glob match (path.Match, not filepath.Match)
	for _, pattern := range rm.patterns {
		if matched, _ := path.Match(pattern, urlPath); matched {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the matcher has no rules configured.
// Safe to call on a nil receiver (returns true).
func (rm *RouteMatcher) IsEmpty() bool {
	if rm == nil {
		return true
	}
	return len(rm.exact) == 0 && len(rm.prefixes) == 0 && len(rm.patterns) == 0
}
