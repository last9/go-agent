package routematcher

import "testing"

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		name     string
		exact    []string
		prefixes []string
		patterns []string
		path     string
		want     bool
	}{
		// Exact matches
		{
			name:  "exact match",
			exact: []string{"/health", "/metrics"},
			path:  "/health",
			want:  true,
		},
		{
			name:  "exact no match",
			exact: []string{"/health"},
			path:  "/healthz",
			want:  false,
		},
		// Prefix matches
		{
			name:     "prefix match",
			prefixes: []string{"/internal/"},
			path:     "/internal/debug",
			want:     true,
		},
		{
			name:     "prefix no match",
			prefixes: []string{"/internal/"},
			path:     "/api/internal",
			want:     false,
		},
		// Glob matches
		{
			name:     "glob match single segment",
			patterns: []string{"/*/health"},
			path:     "/v1/health",
			want:     true,
		},
		{
			name:     "glob no match extra segment",
			patterns: []string{"/*/health"},
			path:     "/v1/api/health",
			want:     false,
		},
		{
			name:     "glob match healthz",
			patterns: []string{"/*/healthz"},
			path:     "/api/healthz",
			want:     true,
		},
		// Combined rules
		{
			name:     "combined: exact wins",
			exact:    []string{"/ping"},
			prefixes: []string{"/debug/"},
			patterns: []string{"/*/metrics"},
			path:     "/ping",
			want:     true,
		},
		{
			name:     "combined: prefix wins",
			exact:    []string{"/ping"},
			prefixes: []string{"/debug/"},
			patterns: []string{"/*/metrics"},
			path:     "/debug/vars",
			want:     true,
		},
		{
			name:     "combined: glob wins",
			exact:    []string{"/ping"},
			prefixes: []string{"/debug/"},
			patterns: []string{"/*/metrics"},
			path:     "/v2/metrics",
			want:     true,
		},
		{
			name:     "combined: no match",
			exact:    []string{"/ping"},
			prefixes: []string{"/debug/"},
			patterns: []string{"/*/metrics"},
			path:     "/api/users",
			want:     false,
		},
		// No false positives
		{
			name:     "no false positive on partial exact",
			exact:    []string{"/health"},
			path:     "/healthy",
			want:     false,
		},
		{
			name:     "no false positive on partial prefix",
			prefixes: []string{"/api/v1/"},
			path:     "/api/v10/foo",
			want:     false,
		},
		// Empty strings are ignored
		{
			name:  "empty exact strings ignored",
			exact: []string{"", "/health", ""},
			path:  "/health",
			want:  true,
		},
		{
			name:     "empty prefix strings ignored",
			prefixes: []string{""},
			path:     "/anything",
			want:     false,
		},
		{
			name:     "empty pattern strings ignored",
			patterns: []string{""},
			path:     "/anything",
			want:     false,
		},
		// Empty matcher
		{
			name: "empty matcher excludes nothing",
			path: "/health",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := New(tt.exact, tt.prefixes, tt.patterns)
			got := rm.ShouldExclude(tt.path)
			if got != tt.want {
				t.Errorf("ShouldExclude(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestShouldExcludeNilReceiver(t *testing.T) {
	var rm *RouteMatcher
	if rm.ShouldExclude("/health") {
		t.Error("nil RouteMatcher should never exclude")
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		exact    []string
		prefixes []string
		patterns []string
		want     bool
	}{
		{"nil slices", nil, nil, nil, true},
		{"empty slices", []string{}, []string{}, []string{}, true},
		{"only empty strings", []string{""}, []string{""}, []string{""}, true},
		{"has exact", []string{"/health"}, nil, nil, false},
		{"has prefix", nil, []string{"/api/"}, nil, false},
		{"has pattern", nil, nil, []string{"/*/health"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := New(tt.exact, tt.prefixes, tt.patterns)
			got := rm.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEmptyNilReceiver(t *testing.T) {
	var rm *RouteMatcher
	if !rm.IsEmpty() {
		t.Error("nil RouteMatcher should be empty")
	}
}
