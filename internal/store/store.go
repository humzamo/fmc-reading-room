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

//go:embed migrations/0002_add_documents_unavailable.sql
var addDocumentsUnavailable string

// UnavailableSourceURL marks a documents row whose file 404s: every
// resolvable field is still recorded (with file_name/file_path left
// empty), so future syncs stop retrying the dead link. Find these with
// `SELECT * FROM documents WHERE source_url = 'file_unavailable'`.
const UnavailableSourceURL = "file_unavailable"

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
	if err := applyVersionedMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// applyVersionedMigrations gates ALTER TABLE migrations (unlike
// 0001_init.sql's CREATE TABLE IF NOT EXISTS, these error if rerun) behind
// SQLite's PRAGMA user_version.
func applyVersionedMigrations(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("store: reading schema version: %w", err)
	}

	migrations := []string{addDocumentsUnavailable}
	for i, migration := range migrations {
		targetVersion := i + 1
		if version >= targetVersion {
			continue
		}
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("store: applying migration %d: %w", targetVersion, err)
		}
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, targetVersion)); err != nil {
			return fmt.Errorf("store: recording schema version %d: %w", targetVersion, err)
		}
	}
	return nil
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
