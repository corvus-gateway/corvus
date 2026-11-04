package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Dialect differences are contained here so the rest of the store can use one
// SQL style (SQLite flavoured, "?" placeholders, LastInsertId):
//   - placeholders "?" are rewritten to "$1..$n" for PostgreSQL
//   - INSERTs into tables with an id column get "RETURNING id" and a synthetic sql.Result
//   - migrations: "INTEGER PRIMARY KEY AUTOINCREMENT" → "BIGSERIAL PRIMARY KEY"

const (
	SQLite   = "sqlite"
	Postgres = "postgres"
)

// tables whose primary key is a generated "id" column
var idTables = map[string]bool{
	"users": true, "certificates": true, "waitrooms": true, "proxy_hosts": true, "config_versions": true,
	"audit_log": true, "rate_limits": true, "access_lists": true, "access_list_users": true,
	"alert_channels": true, "alert_history": true,
	"security_rules": true, "security_rule_hosts": true, "ip_lists": true, "ip_bans": true,
	"api_tokens": true, "redirection_hosts": true,
}

type pgQuerier struct{ q querier }

func rebind(query string) string {
	var b strings.Builder
	n := 0
	inQuote := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '\'' {
			inQuote = !inQuote
		}
		if c == '?' && !inQuote {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

type idResult struct {
	id       int64
	affected int64
}

func (r idResult) LastInsertId() (int64, error) { return r.id, nil }
func (r idResult) RowsAffected() (int64, error) { return r.affected, nil }

func insertTable(query string) string {
	q := strings.TrimSpace(query)
	if len(q) < 12 || !strings.EqualFold(q[:12], "INSERT INTO ") {
		return ""
	}
	rest := strings.TrimSpace(q[12:])
	end := strings.IndexAny(rest, " (")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func (p pgQuerier) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if t := insertTable(query); idTables[t] && !strings.Contains(strings.ToUpper(query), "ON CONFLICT") {
		var id int64
		if err := p.q.QueryRowContext(ctx, rebind(query)+" RETURNING id", args...).Scan(&id); err != nil {
			return nil, err
		}
		return idResult{id: id, affected: 1}, nil
	}
	return p.q.ExecContext(ctx, rebind(query), args...)
}

func (p pgQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.q.QueryContext(ctx, rebind(query), args...)
}

func (p pgQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.q.QueryRowContext(ctx, rebind(query), args...)
}

func (s *Store) wrap(q querier) querier {
	if s.dialect == Postgres {
		return pgQuerier{q}
	}
	return q
}

func migrationSQL(dialect, body string) string {
	if dialect == Postgres {
		body = strings.ReplaceAll(body, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY")
	}
	return body
}

// IsUnique reports whether err is a unique-constraint violation (either engine).
func IsUnique(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "UNIQUE constraint") || strings.Contains(m, "duplicate key value") || strings.Contains(m, "SQLSTATE 23505")
}

// Dialect returns the database engine in use.
func (s *Store) Dialect() string { return s.dialect }
