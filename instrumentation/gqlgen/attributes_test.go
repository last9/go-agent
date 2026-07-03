package gqlgen

import (
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

func opContext(operation ast.Operation, name, rawQuery string) *graphql.OperationContext {
	return &graphql.OperationContext{
		RawQuery: rawQuery,
		Operation: &ast.OperationDefinition{
			Operation: operation,
			Name:      name,
		},
	}
}

func TestSpanName(t *testing.T) {
	tests := []struct {
		name string
		oc   *graphql.OperationContext
		want string
	}{
		{"query with a name", opContext(ast.Query, "GetUser", ""), "query GetUser"},
		{"mutation with a name", opContext(ast.Mutation, "CreateTeam", ""), "mutation CreateTeam"},
		{"anonymous operation", opContext(ast.Query, "", ""), "GraphQL Operation"},
		{"nil operation context", nil, "GraphQL Operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spanName(tt.oc); got != tt.want {
				t.Errorf("spanName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOperationTypeAttribute(t *testing.T) {
	tests := []struct {
		name string
		oc   *graphql.OperationContext
		want string
	}{
		{"query", opContext(ast.Query, "GetUser", ""), "query"},
		{"mutation", opContext(ast.Mutation, "CreateTeam", ""), "mutation"},
		{"subscription", opContext(ast.Subscription, "OnMessage", ""), "subscription"},
		{"nil operation context defaults to query", nil, "query"},
		{"nil Operation field defaults to query", &graphql.OperationContext{}, "query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := operationTypeAttribute(tt.oc).Value.AsString()
			if got != tt.want {
				t.Errorf("operationTypeAttribute() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsSubscription(t *testing.T) {
	if !isSubscription(opContext(ast.Subscription, "OnMessage", "")) {
		t.Error("expected subscription operation to report true")
	}
	if isSubscription(opContext(ast.Query, "GetUser", "")) {
		t.Error("expected query operation to report false")
	}
	if isSubscription(nil) {
		t.Error("expected nil operation context to report false, not panic")
	}
	if isSubscription(&graphql.OperationContext{}) {
		t.Error("expected operation context with nil Operation field to report false, not panic")
	}
}

func TestBaseAttributes_Redaction(t *testing.T) {
	oc := opContext(ast.Query, "GetUser", "query GetUser { user { email } }")

	t.Run("IncludeQueryDocument=false excludes graphql.document", func(t *testing.T) {
		attrs := baseAttributes(oc, false)
		for _, a := range attrs {
			if string(a.Key) == "graphql.document" {
				t.Fatal("expected graphql.document to be excluded when IncludeQueryDocument is false")
			}
		}
	})

	t.Run("IncludeQueryDocument=true includes graphql.document with raw query text", func(t *testing.T) {
		attrs := baseAttributes(oc, true)
		found := false
		for _, a := range attrs {
			if string(a.Key) == "graphql.document" {
				found = true
				if a.Value.AsString() != oc.RawQuery {
					t.Errorf("graphql.document = %q, want %q", a.Value.AsString(), oc.RawQuery)
				}
			}
		}
		if !found {
			t.Fatal("expected graphql.document to be present when IncludeQueryDocument is true")
		}
	})

	t.Run("nil operation context with IncludeQueryDocument=true does not panic", func(t *testing.T) {
		attrs := baseAttributes(nil, true)
		for _, a := range attrs {
			if string(a.Key) == "graphql.document" {
				t.Fatal("expected no graphql.document attribute for a nil operation context")
			}
		}
	})

	t.Run("oversized query document is truncated", func(t *testing.T) {
		big := opContext(ast.Query, "GetUser", strings.Repeat("a", maxCapturedTextLen+100))
		attrs := baseAttributes(big, true)
		for _, a := range attrs {
			if string(a.Key) == "graphql.document" {
				if len(a.Value.AsString()) > maxCapturedTextLen+len("...(truncated)") {
					t.Errorf("graphql.document not truncated, len=%d", len(a.Value.AsString()))
				}
			}
		}
	})

	t.Run("always includes operation name and type", func(t *testing.T) {
		attrs := baseAttributes(oc, false)
		var hasName, hasType bool
		for _, a := range attrs {
			switch string(a.Key) {
			case "graphql.operation.name":
				hasName = a.Value.AsString() == "GetUser"
			case "graphql.operation.type":
				hasType = a.Value.AsString() == "query"
			}
		}
		if !hasName {
			t.Error("expected graphql.operation.name=GetUser")
		}
		if !hasType {
			t.Error("expected graphql.operation.type=query")
		}
	})
}
