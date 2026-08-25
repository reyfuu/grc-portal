package db

import (
	"database/sql"
	_ "embed"
)

//go:embed schema.sql
var schemaSQL string

// Migrate runs the embedded schema. It is idempotent (CREATE ... IF NOT EXISTS).
// pgx's stdlib driver runs argument-less Exec via the simple protocol, so the
// multi-statement schema executes in one call.
func Migrate(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}
