// Package gqlgen provides Last9 instrumentation for the gqlgen GraphQL server library.
package gqlgen

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

// operationTypeAttribute maps a gqlgen operation context to the OTel semconv
// operation-type attribute. Defaults to query when the operation is nil or
// carries an unrecognized type.
func operationTypeAttribute(oc *graphql.OperationContext) attribute.KeyValue {
	if oc == nil || oc.Operation == nil {
		return semconv.GraphqlOperationTypeQuery
	}
	switch oc.Operation.Operation {
	case ast.Mutation:
		return semconv.GraphqlOperationTypeMutation
	case ast.Subscription:
		return semconv.GraphqlOperationTypeSubscription
	default:
		return semconv.GraphqlOperationTypeQuery
	}
}

// isSubscription reports whether the operation context describes a GraphQL
// subscription. Subscriptions are not instrumented: gqlgen invokes
// ResponseInterceptor once per streamed message for a subscription's
// lifetime, not once per operation, so spanning them would create one span
// per message rather than one span per operation.
func isSubscription(oc *graphql.OperationContext) bool {
	return oc != nil && oc.Operation != nil && oc.Operation.Operation == ast.Subscription
}

// operationName returns the GraphQL operation name, or "" when the operation
// is anonymous/unnamed. The name is client-supplied and always-on (unlike
// graphql.document), so it is truncated the same way to prevent an
// oversized operation name from growing every span regardless of
// IncludeQueryDocument.
func operationName(oc *graphql.OperationContext) string {
	if oc == nil || oc.Operation == nil {
		return ""
	}
	return truncate(oc.Operation.Name)
}

// spanName builds the INTERNAL span name as "{operationType} {operationName}"
// (e.g. "query GetUser"), falling back to "GraphQL Operation" when the
// operation is anonymous.
func spanName(oc *graphql.OperationContext) string {
	name := operationName(oc)
	if name == "" {
		return "GraphQL Operation"
	}
	return string(operationTypeAttribute(oc).Value.AsString()) + " " + name
}

// baseAttributes builds the always-on span attributes (operation name and
// type). When includeQueryDocument is true, the raw GraphQL query document
// text is also included via graphql.document.
//
// includeQueryDocument gates a sensitive-data attribute: GraphQL query
// documents can carry business-sensitive field selections. Disable in
// production environments that handle PII or sensitive data.
func baseAttributes(oc *graphql.OperationContext, includeQueryDocument bool) []attribute.KeyValue {
	attrs := []attribute.KeyValue{operationTypeAttribute(oc)}
	if name := operationName(oc); name != "" {
		attrs = append(attrs, semconv.GraphqlOperationName(name))
	}
	if includeQueryDocument && oc != nil {
		attrs = append(attrs, semconv.GraphqlDocument(truncate(oc.RawQuery)))
	}
	return attrs
}
