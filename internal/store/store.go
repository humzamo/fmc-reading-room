// Package store persists everything the sync process learns about
// proceedings and documents in a single SQLite database, and doubles as the
// app's run log (the sync_runs table answers "when did this last run, and
// what happened").
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/0001_init.sql
var initSchema string

// Store wraps the SQLite database. It's safe for concurrent use by multiple
// goroutines (database/sql pools connections internally); SQLite itself
// serializes writes.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and
// applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: opening %s: %w", path, err)
	}
	// SQLite only really supports one writer at a time; a single
	// connection avoids "database is locked" errors under concurrent
	// access from this process.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(initSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: applying schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

const dateLayout = "2006-01-02"

func nullableDateString(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(dateLayout)
}

func nullableTimeString(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339)
}

func parseNullableDate(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(dateLayout, s.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseNullableTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return time.Time{}
	}
	return t
}
