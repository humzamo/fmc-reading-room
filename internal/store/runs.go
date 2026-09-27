package store

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
)

// SyncRun mirrors the sync_runs table: the app's run log. The latest row is
// the "when did this last run, and what happened" status the user asked
// for.
type SyncRun struct {
	ID                  int64
	StartedAt           time.Time
	FinishedAt          time.Time
	Status              string
	ProceedingsScanned  int
	NewProceedings      int
	NewDocuments        int
	DocumentsDownloaded int
	// DocumentsUnavailable counts documents the site's own listing links to
	// that returned 404. These are recorded (see UnavailableSourceURL) so
	// they're never retried automatically; DocumentsFailed below is for
	// everything else (still worth retrying next run).
	DocumentsUnavailable int
	DocumentsFailed      int
	Error                string
}

// StartRun records the beginning of a sync run and returns its ID.
func (s *Store) StartRun(startedAt time.Time) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO sync_runs (started_at, status) VALUES (?, ?)`,
		startedAt.Format(time.RFC3339), RunStatusRunning)
	if err != nil {
		return 0, fmt.Errorf("store: starting run: %w", err)
	}
	return res.LastInsertId()
}

// FinishRun finalizes a run with its outcome and counters.
func (s *Store) FinishRun(id int64, finishedAt time.Time, status string, scanned, newProceedings, newDocuments, downloaded, unavailable, failed int, runErr string) error {
	_, err := s.db.Exec(`
		UPDATE sync_runs
		SET finished_at = ?, status = ?, proceedings_scanned = ?, new_proceedings = ?,
		    new_documents = ?, documents_downloaded = ?, documents_unavailable = ?, documents_failed = ?, error = ?
		WHERE id = ?`,
		finishedAt.Format(time.RFC3339), status, scanned, newProceedings, newDocuments, downloaded, unavailable, failed, nullString(runErr), id)
	if err != nil {
		return fmt.Errorf("store: finishing run %d: %w", id, err)
	}
	return nil
}

// LatestRun returns the most recently started run, or ok=false if none
// exist yet.
func (s *Store) LatestRun() (SyncRun, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, started_at, finished_at, status, proceedings_scanned, new_proceedings,
		       new_documents, documents_downloaded, documents_unavailable, documents_failed, error
		FROM sync_runs ORDER BY id DESC LIMIT 1`)

	var run SyncRun
	var startedAt string
	var finishedAt, runErr sql.NullString
	err := row.Scan(&run.ID, &startedAt, &finishedAt, &run.Status, &run.ProceedingsScanned,
		&run.NewProceedings, &run.NewDocuments, &run.DocumentsDownloaded, &run.DocumentsUnavailable, &run.DocumentsFailed, &runErr)
	if err == sql.ErrNoRows {
		return SyncRun{}, false, nil
	}
	if err != nil {
		return SyncRun{}, false, fmt.Errorf("store: getting latest run: %w", err)
	}
	run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	run.FinishedAt = parseNullableTime(finishedAt)
	run.Error = runErr.String
	return run, true, nil
}
