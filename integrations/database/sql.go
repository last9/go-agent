package database

import (
	"strings"
	"sync"
)

// parsedSQL holds the cached result of a parseSQL call.
type parsedSQL struct {
	operation, table string
}

// parsedQueryCache avoids re-parsing the same query string on every execution.
// SQL queries in a real application are a bounded, repeatedly-used set so
// the map grows to a stable size and stays there.
var parsedQueryCache sync.Map // map[string]*parsedSQL

// parseSQLCached returns the same result as parseSQL but caches by query string,
// eliminating redundant token-splitting on the per-request hot path.
func parseSQLCached(query string) (string, string) {
	if v, ok := parsedQueryCache.Load(query); ok {
		if p, ok := v.(*parsedSQL); ok {
			return p.operation, p.table
		}
	}
	op, tbl := parseSQL(query)
	parsedQueryCache.Store(query, &parsedSQL{op, tbl})
	return op, tbl
}

// parseSQL extracts the SQL operation (verb) and primary table name from a
// query string using lightweight token-based parsing — no grammar is involved.
//
// Handles SELECT, INSERT, UPDATE, DELETE, REPLACE, TRUNCATE, CREATE, DROP,
// ALTER. Returns empty strings for unrecognised or unparseable input.
// Table names preserve their original case from the query.
func parseSQL(query string) (operation, table string) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return
	}

	// Keep two parallel slices: original case for value extraction,
	// uppercase for keyword matching.
	raw := strings.Fields(trimmed)
	upper := make([]string, len(raw))
	for i, f := range raw {
		upper[i] = strings.ToUpper(f)
	}

	op := upper[0]
	if !knownVerb(op) {
		return
	}
	operation = op
	table = extractTable(upper, raw, op)
	return
}

// knownVerb reports whether op is a recognised SQL statement verb.
func knownVerb(op string) bool {
	switch op {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "REPLACE",
		"TRUNCATE", "CREATE", "DROP", "ALTER":
		return true
	}
	return false
}

// extractTable returns the primary table name for the given SQL operation.
func extractTable(upper, raw []string, operation string) string {
	switch operation {
	case "SELECT", "DELETE":
		return tokenAfterKeyword(upper, raw, "FROM")
	case "INSERT", "REPLACE":
		return tokenAfterKeyword(upper, raw, "INTO")
	case "UPDATE":
		return updateTable(upper, raw)
	case "TRUNCATE":
		return truncateTable(upper, raw)
	case "CREATE", "DROP", "ALTER":
		if len(raw) > 2 {
			return cleanIdent(raw[2])
		}
	}
	return ""
}

// updateTable extracts the table from an UPDATE statement, skipping optional
// modifier keywords (LOW_PRIORITY, IGNORE, ONLY).
func updateTable(upper, raw []string) string {
	for i, u := range upper[1:] {
		if u == "LOW_PRIORITY" || u == "IGNORE" || u == "ONLY" {
			continue
		}
		return cleanIdent(raw[i+1])
	}
	return ""
}

// truncateTable extracts the table from a TRUNCATE [TABLE] statement.
func truncateTable(upper, raw []string) string {
	if len(raw) < 2 {
		return ""
	}
	idx := 1
	if upper[1] == "TABLE" {
		idx = 2
	}
	if idx < len(raw) {
		return cleanIdent(raw[idx])
	}
	return ""
}

// tokenAfterKeyword returns the original-case token that immediately follows
// the first occurrence of keyword (matched case-insensitively via upper).
// Subquery openers (tokens starting with "(", and "SELECT"/"WITH") are skipped
// so that e.g. "FROM (SELECT …) alias" searches past the inner query.
func tokenAfterKeyword(upper, raw []string, keyword string) string {
	for i, u := range upper {
		if u != keyword || i+1 >= len(raw) {
			continue
		}
		next := upper[i+1]
		if strings.HasPrefix(next, "(") || next == "SELECT" || next == "WITH" {
			continue
		}
		return cleanIdent(raw[i+1])
	}
	return ""
}

// cleanIdent strips SQL quoting characters and schema prefixes from an
// identifier token, e.g. `"public"."users"` → `users`.
func cleanIdent(s string) string {
	s = strings.TrimLeft(s, "`\"[")
	s = strings.TrimRight(s, "`\"]),;")
	// schema.table → table
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		s = s[idx+1:]
	}
	return s
}
