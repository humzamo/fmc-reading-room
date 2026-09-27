package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Proceeding mirrors the proceedings table. FolderName is set once, on
// first insert, and never changed even if Title later differs on the site.
type Proceeding struct {
	ProceedingNumber string
	Title            string
	FolderName       string
	CreatedDate      time.Time
	LastUpdated      time.Time
	ProceedingType   string // "" if unknown
	IsClosed         bool
	LastSyncedAt     time.Time
}

// GetProceeding returns the stored proceeding, or ok=false if it doesn't
// exist yet.
func (s *Store) GetProceeding(number string) (Proceeding, bool, error) {
	row := s.db.QueryRow(`
		SELECT proceeding_number, title, folder_name, created_date, last_updated,
		       proceeding_type, is_closed, last_synced_at
		FROM proceedings WHERE proceeding_number = ?`, number)

	var p Proceeding
	var createdDate, lastUpdated, lastSyncedAt sql.NullString
	var proceedingType sql.NullString
	var isClosed int
	err := row.Scan(&p.ProceedingNumber, &p.Title, &p.FolderName, &createdDate, &lastUpdated,
		&proceedingType, &isClosed, &lastSyncedAt)
	if err == sql.ErrNoRows {
		return Proceeding{}, false, nil
	}
	if err != nil {
		return Proceeding{}, false, fmt.Errorf("store: getting proceeding %s: %w", number, err)
	}
	p.CreatedDate = parseNullableDate(createdDate)
	p.LastUpdated = parseNullableDate(lastUpdated)
	p.LastSyncedAt = parseNullableTime(lastSyncedAt)
	p.ProceedingType = proceedingType.String
	p.IsClosed = isClosed != 0
	return p, true, nil
}

// InsertProceeding adds a brand-new proceeding, including its permanent
// FolderName.
func (s *Store) InsertProceeding(p Proceeding) error {
	_, err := s.db.Exec(`
		INSERT INTO proceedings
			(proceeding_number, title, folder_name, created_date, last_updated,
			 proceeding_type, is_closed, last_synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ProceedingNumber, p.Title, p.FolderName, nullableDateString(p.CreatedDate), nullableDateString(p.LastUpdated),
		nullString(p.ProceedingType), boolToInt(p.IsClosed), nullableTimeString(p.LastSyncedAt))
	if err != nil {
		return fmt.Errorf("store: inserting proceeding %s: %w", p.ProceedingNumber, err)
	}
	return nil
}

// UpdateProceedingMetadata refreshes the bookkeeping fields discovered by a
// search crawl (title text, created date, type, closed status).
// FolderName is intentionally not a parameter: it must never change.
func (s *Store) UpdateProceedingMetadata(number, title string, createdDate time.Time, proceedingType string, isClosed bool) error {
	_, err := s.db.Exec(`
		UPDATE proceedings
		SET title = ?, created_date = ?, proceeding_type = ?, is_closed = ?
		WHERE proceeding_number = ?`,
		title, nullableDateString(createdDate), nullString(proceedingType), boolToInt(isClosed), number)
	if err != nil {
		return fmt.Errorf("store: updating proceeding %s: %w", number, err)
	}
	return nil
}

// UpdateProceedingSyncState records the outcome of fetching this
// proceeding's detail page.
func (s *Store) UpdateProceedingSyncState(number string, lastUpdated, syncedAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE proceedings SET last_updated = ?, last_synced_at = ? WHERE proceeding_number = ?`,
		nullableDateString(lastUpdated), nullableTimeString(syncedAt), number)
	if err != nil {
		return fmt.Errorf("store: updating sync state for %s: %w", number, err)
	}
	return nil
}

// ListProceedingNumbers returns every proceeding number currently stored.
func (s *Store) ListProceedingNumbers() ([]string, error) {
	rows, err := s.db.Query(`SELECT proceeding_number FROM proceedings ORDER BY proceeding_number`)
	if err != nil {
		return nil, fmt.Errorf("store: listing proceedings: %w", err)
	}
	defer rows.Close()

	var numbers []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scanning proceeding number: %w", err)
		}
		numbers = append(numbers, n)
	}
	return numbers, rows.Err()
}

// CountProceedings returns the total number of stored proceedings.
func (s *Store) CountProceedings() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM proceedings`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: counting proceedings: %w", err)
	}
	return n, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
