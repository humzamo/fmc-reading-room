package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Document mirrors the documents table. FileName/FilePath are set once,
// at download time, and never change afterwards (same rationale as
// Proceeding.FolderName).
type Document struct {
	UniqueKey        string
	ProceedingNumber string
	DocumentNumber   int
	ServedDate       time.Time
	Description      string
	SourceURL        string
	FileName         string
	FilePath         string
	ContentType      string
	FileSize         int64
	DownloadedAt     time.Time
}

// UniqueKey builds the "{proceeding_number}_{document_number}" key.
func UniqueKey(proceedingNumber string, documentNumber int) string {
	return fmt.Sprintf("%s_%d", proceedingNumber, documentNumber)
}

// ExistingDocumentNumbers returns the set of document numbers already
// stored for a proceeding, used to diff against the site's current list and
// find what's new.
func (s *Store) ExistingDocumentNumbers(proceedingNumber string) (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT document_number FROM documents WHERE proceeding_number = ?`, proceedingNumber)
	if err != nil {
		return nil, fmt.Errorf("store: listing documents for %s: %w", proceedingNumber, err)
	}
	defer rows.Close()

	existing := make(map[int]bool)
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scanning document number: %w", err)
		}
		existing[n] = true
	}
	return existing, rows.Err()
}

// DocumentExists reports whether a document is already recorded, by its
// unique key. Used by the --since path (internal/sync), which discovers
// documents flatly across all proceedings via DocumentSearch rather than
// grouped per-proceeding, so a per-proceeding existing-set doesn't apply.
func (s *Store) DocumentExists(proceedingNumber string, documentNumber int) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM documents WHERE unique_key = ?`, UniqueKey(proceedingNumber, documentNumber)).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: checking document %s_%d: %w", proceedingNumber, documentNumber, err)
	}
	return true, nil
}

// InsertDocument records a newly downloaded document.
func (s *Store) InsertDocument(d Document) error {
	_, err := s.db.Exec(`
		INSERT INTO documents
			(unique_key, proceeding_number, document_number, served_date, description,
			 source_url, file_name, file_path, content_type, file_size, downloaded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.UniqueKey, d.ProceedingNumber, d.DocumentNumber, nullableDateString(d.ServedDate), d.Description,
		d.SourceURL, d.FileName, d.FilePath, nullString(d.ContentType), d.FileSize, nullableTimeString(d.DownloadedAt))
	if err != nil {
		return fmt.Errorf("store: inserting document %s: %w", d.UniqueKey, err)
	}
	return nil
}

// CountDocuments returns the total number of stored documents.
func (s *Store) CountDocuments() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM documents`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: counting documents: %w", err)
	}
	return n, nil
}
