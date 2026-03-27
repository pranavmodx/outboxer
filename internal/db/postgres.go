package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

// Connect opens a connection to PostgreSQL and verifies it with a ping.
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db.Ping: %w", err)
	}
	log.Println("✅ Connected to PostgreSQL")
	return db, nil
}

// Migrate creates the orders and outbox_events tables if they don't exist.
func Migrate(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS orders (
			id            TEXT PRIMARY KEY,
			customer_name TEXT NOT NULL,
			product       TEXT NOT NULL,
			quantity      INT  NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS outbox_events (
			id             TEXT PRIMARY KEY,
			aggregate_type TEXT        NOT NULL,
			aggregate_id   TEXT        NOT NULL,
			event_type     TEXT        NOT NULL,
			payload        JSONB       NOT NULL,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			status         TEXT        NOT NULL DEFAULT 'PENDING'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_outbox_pending
			ON outbox_events (created_at)
			WHERE status = 'PENDING'`,
		`CREATE TABLE IF NOT EXISTS processed_events (
			event_id      TEXT PRIMARY KEY,
			aggregate_id  TEXT NOT NULL,
			event_type    TEXT NOT NULL,
			consumer_name TEXT NOT NULL,
			processed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE processed_events ADD COLUMN IF NOT EXISTS aggregate_id TEXT DEFAULT ''`,
		`ALTER TABLE processed_events ADD COLUMN IF NOT EXISTS event_type TEXT DEFAULT ''`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	log.Println("✅ Database migrated")
	return nil
}
