package database

import (
	"fmt"
)

// CaseInsensitiveLike returns a SQL fragment that performs a
// case-insensitive pattern match on the given column, shaped for the
// active database dialect.
//
//   - postgres: "col ILIKE ?"  - preserved so the existing pg_trgm GIN
//     indexes on the affected columns stay usable. Switching PG to
//     LOWER() LIKE LOWER() would require expression-index rebuilds and
//     could regress search performance.
//   - every other dialect (mysql, sqlite, anything future): the portable
//     "LOWER(col) LIKE LOWER(?)" form.
func CaseInsensitiveLike(dialectName, column, placeholder string) string {
	if dialectName == "postgres" {
		return column + " ILIKE " + placeholder
	}
	return "LOWER(" + column + ") LIKE LOWER(" + placeholder + ")"
}

// JSONPathExpr returns a SQL fragment that extracts a scalar value from
// a JSON column at the given key, shaped for the active database dialect.
//
//   - postgres: "col ->> 'key'"              (jsonb text extraction)
//   - mysql:    "col ->> '$.key'"            (MySQL 8.0+ JSON path)
//   - sqlite:   "json_extract(col, '$.key')" (portable across SQLite versions)
//
// key must be a simple identifier matching [a-zA-Z0-9_-]. Dotted paths
// and metacharacters are rejected with an error rather than silently
// stripped — silently stripping turned "a.b" into "ab", which queried
// a different key than the caller intended.
func JSONPathExpr(dialectName, column, key string) (string, error) {
	if err := validateJSONPathKey(key); err != nil {
		return "", err
	}
	switch dialectName {
	case "postgres":
		return fmt.Sprintf("%s ->> '%s'", column, key), nil
	case "mysql":
		return fmt.Sprintf("%s ->> '$.%s'", column, key), nil
	default:
		return fmt.Sprintf("json_extract(%s, '$.%s')", column, key), nil
	}
}

// mysqlMaterializedJSONColumns maps a (JSON column, key) extraction to the
// generated column that materializes and indexes it in the MySQL schema.
//
// MySQL only substitutes an indexed generated column for equality-shaped
// predicates (=, <, IN, BETWEEN, ...), never for LIKE. A LIKE-prefix scan
// therefore has to name the generated column itself to stay on the index.
// Keeping the mapping here means the schema/query coupling is stated once,
// instead of a bare column name appearing at each call site where nothing
// would flag it if the migration renamed the column.
var mysqlMaterializedJSONColumns = map[[2]string]string{
	{"metadata", "external_id"}: "metadata_external_id",
}

// JSONPathExprIndexed behaves like JSONPathExpr, except it returns the
// materialized generated column when the active dialect has one for this
// (column, key). Prefer it over JSONPathExpr wherever the predicate is
// expected to use an index.
func JSONPathExprIndexed(dialectName, column, key string) (string, error) {
	if err := validateJSONPathKey(key); err != nil {
		return "", err
	}
	if dialectName == "mysql" {
		if generated, ok := mysqlMaterializedJSONColumns[[2]string{column, key}]; ok {
			return generated, nil
		}
	}
	return JSONPathExpr(dialectName, column, key)
}

// validateJSONPathKey ensures key is a simple identifier ([a-zA-Z0-9_-]+).
// Empty keys and anything containing JSON path metacharacters (., [], *,
// quotes, etc.) are rejected. This prevents both injection and silent
// key corruption.
func validateJSONPathKey(key string) error {
	if key == "" {
		return fmt.Errorf("JSON path key must not be empty")
	}
	for _, r := range key {
		if r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			continue
		}
		return fmt.Errorf("JSON path key %q contains invalid character %q; only [a-zA-Z0-9_-] are allowed", key, string(r))
	}
	return nil
}
