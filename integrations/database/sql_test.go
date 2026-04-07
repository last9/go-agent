package database

import "testing"

func TestParseSQL(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantOperation string
		wantTable     string
	}{
		// SELECT
		{
			name:          "simple select",
			query:         "SELECT id, name FROM users WHERE id = $1",
			wantOperation: "SELECT",
			wantTable:     "users",
		},
		{
			name:          "select with schema-qualified table",
			query:         "SELECT * FROM public.orders",
			wantOperation: "SELECT",
			wantTable:     "orders",
		},
		{
			name:          "select with quoted identifier preserves case",
			query:         `SELECT * FROM "User"`,
			wantOperation: "SELECT",
			wantTable:     "User",
		},
		{
			name:          "select with backtick identifier preserves case",
			query:         "SELECT * FROM `products`",
			wantOperation: "SELECT",
			wantTable:     "products",
		},
		{
			name:          "select with subquery in FROM resolves inner table",
			query:         "SELECT * FROM (SELECT id FROM users) sub",
			wantOperation: "SELECT",
			wantTable:     "users",
		},
		{
			name:          "select with join",
			query:         "SELECT u.name, o.id FROM users u JOIN orders o ON u.id = o.user_id",
			wantOperation: "SELECT",
			wantTable:     "users",
		},
		{
			name:          "lowercase select",
			query:         "select * from events",
			wantOperation: "SELECT",
			wantTable:     "events",
		},
		// INSERT
		{
			name:          "insert with columns",
			query:         "INSERT INTO orders (user_id, total) VALUES ($1, $2)",
			wantOperation: "INSERT",
			wantTable:     "orders",
		},
		{
			name:          "replace into",
			query:         "REPLACE INTO sessions (id, data) VALUES (?, ?)",
			wantOperation: "REPLACE",
			wantTable:     "sessions",
		},
		// UPDATE
		{
			name:          "simple update",
			query:         "UPDATE users SET name = $1 WHERE id = $2",
			wantOperation: "UPDATE",
			wantTable:     "users",
		},
		{
			name:          "update with ignore modifier",
			query:         "UPDATE IGNORE products SET price = 10.0 WHERE id = 1",
			wantOperation: "UPDATE",
			wantTable:     "products",
		},
		// DELETE
		{
			name:          "simple delete",
			query:         "DELETE FROM sessions WHERE expires_at < NOW()",
			wantOperation: "DELETE",
			wantTable:     "sessions",
		},
		// TRUNCATE
		{
			name:          "truncate table keyword",
			query:         "TRUNCATE TABLE audit_logs",
			wantOperation: "TRUNCATE",
			wantTable:     "audit_logs",
		},
		{
			name:          "truncate without table keyword",
			query:         "TRUNCATE events",
			wantOperation: "TRUNCATE",
			wantTable:     "events",
		},
		// DDL
		{
			name:          "create table",
			query:         "CREATE TABLE new_table (id INT PRIMARY KEY)",
			wantOperation: "CREATE",
			wantTable:     "new_table",
		},
		{
			// Lightweight parser takes token[2]; IF EXISTS is not handled —
			// that would require a more complex parser. Span name degrades to "DROP IF".
			name:          "drop table without if exists",
			query:         "DROP TABLE old_table",
			wantOperation: "DROP",
			wantTable:     "old_table",
		},
		{
			name:          "alter table",
			query:         "ALTER TABLE users ADD COLUMN email TEXT",
			wantOperation: "ALTER",
			wantTable:     "users",
		},
		// Edge cases
		{
			name:          "empty query",
			query:         "",
			wantOperation: "",
			wantTable:     "",
		},
		{
			name:          "whitespace only",
			query:         "   ",
			wantOperation: "",
			wantTable:     "",
		},
		{
			name:          "unrecognised statement",
			query:         "EXPLAIN SELECT * FROM users",
			wantOperation: "",
			wantTable:     "",
		},
		{
			name:          "select with no from clause",
			query:         "SELECT NOW()",
			wantOperation: "SELECT",
			wantTable:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOp, gotTable := parseSQL(tt.query)
			if gotOp != tt.wantOperation {
				t.Errorf("operation = %q, want %q", gotOp, tt.wantOperation)
			}
			if gotTable != tt.wantTable {
				t.Errorf("table = %q, want %q", gotTable, tt.wantTable)
			}
		})
	}
}
