// Package sync orchestrates one full pass over the FMC reading room: it
// discovers every proceeding, upserts its bookkeeping fields, then finds
// and downloads whatever documents aren't already on disk. See the project
// plan for why this is idempotent and diff-based rather than date-filtered
// by default, and Options.Since below for the (safe) date-filtered fast
// path.
package sync

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/humzamoazzam/fmc-reading-room/internal/fmcsite"
	"github.com/humzamoazzam/fmc-reading-room/internal/store"
)

// Options configures a sync run.
type Options struct {
	// ProceedingsDir is where per-proceeding folders are created (the
	// "proceedings" directory).
	ProceedingsDir string
	// Since, if set, switches document discovery from fetching every
	// proceeding's detail page to one site-wide DocumentSearch query
	// filtered by "served on or after this date" — much cheaper when only
	// a small delta is expected. This is safe regardless of proceeding
	// age: the filter is on each document's own serve date, not on when
	// its proceeding was created, so a brand-new filing on an old,
	// otherwise-dormant proceeding is found just as reliably as one on a
	// proceeding created yesterday. Proceeding discovery/metadata (title,
	// type, closed) is unaffected — that pass always runs in full either
	// way, since it's cheap on its own.
	Since time.Time
	// Limit caps how much work the document-fetching phase does — the
	// number of proceedings checked (default mode) or the number of
	// search result rows considered (Since mode) — for smoke-testing
	// against the live site without doing a full crawl. Proceeding
	// discovery/metadata is unaffected. 0 = no limit.
	Limit int
	// DryRun discovers and diffs normally but skips writing files and
	// documents rows, only logging what would happen.
	DryRun bool
	// Concurrency bounds how many proceedings, or in Since mode how many
	// individual documents, are processed at once. Defaults to 5.
	Concurrency int
	// Progress, if set, receives human-readable progress messages.
	Progress func(format string, args ...any)
}

func (o Options) progress(format string, args ...any) {
	if o.Progress != nil {
		o.Progress(format, args...)
	}
}

// Syncer runs sync passes against a site client and a store.
type Syncer struct {
	Client *fmcsite.Client
	Store  *store.Store
	Opts   Options
}

// New builds a Syncer, applying option defaults.
func New(client *fmcsite.Client, st *store.Store, opts Options) *Syncer {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 5
	}
	if opts.Progress != nil {
		client.Progress = opts.Progress
	}
	return &Syncer{Client: client, Store: st, Opts: opts}
}

// counters accumulates one run's outcome across concurrent goroutines.
type counters struct {
	scanned, newDocuments, downloaded, unavailable, failed atomic.Int64
}

// Run performs one full sync pass and returns its logged summary.
func (s *Syncer) Run(ctx context.Context) (store.SyncRun, error) {
	startedAt := time.Now()
	runID, err := s.Store.StartRun(startedAt)
	if err != nil {
		return store.SyncRun{}, err
	}

	discovered, err := s.Client.DiscoverProceedings(ctx)
	if err != nil {
		return store.SyncRun{}, s.failRun(runID, startedAt, err)
	}
	s.Opts.progress("discovered %d proceedings", len(discovered))

	folderNames, newProceedings, err := s.upsertProceedings(discovered)
	if err != nil {
		return store.SyncRun{}, s.failRun(runID, startedAt, err)
	}

	var c counters
	if s.Opts.Since.IsZero() {
		if err := s.syncAllProceedings(ctx, discovered, folderNames, &c); err != nil {
			return store.SyncRun{}, s.failRun(runID, startedAt, err)
		}
	} else {
		s.Opts.progress("using DocumentSearch since %s instead of checking every proceeding individually", s.Opts.Since.Format("2006-01-02"))
		if err := s.syncSince(ctx, s.Opts.Since, folderNames, &c); err != nil {
			return store.SyncRun{}, s.failRun(runID, startedAt, err)
		}
	}

	finishedAt := time.Now()
	run := store.SyncRun{
		ID:                   runID,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		Status:               store.RunStatusSuccess,
		ProceedingsScanned:   int(c.scanned.Load()),
		NewProceedings:       newProceedings,
		NewDocuments:         int(c.newDocuments.Load()),
		DocumentsDownloaded:  int(c.downloaded.Load()),
		DocumentsUnavailable: int(c.unavailable.Load()),
		DocumentsFailed:      int(c.failed.Load()),
	}
	if err := s.Store.FinishRun(runID, finishedAt, run.Status, run.ProceedingsScanned, run.NewProceedings,
		run.NewDocuments, run.DocumentsDownloaded, run.DocumentsUnavailable, run.DocumentsFailed, ""); err != nil {
		return store.SyncRun{}, err
	}
	return run, nil
}

// failRun marks the run as failed and returns the original error, so a
// caller sees the same failure while the DB never keeps a run stuck as
// "running" forever.
func (s *Syncer) failRun(runID int64, startedAt time.Time, cause error) error {
	if err := s.Store.FinishRun(runID, time.Now(), store.RunStatusFailed, 0, 0, 0, 0, 0, 0, cause.Error()); err != nil {
		return fmt.Errorf("%w (also failed to record run failure: %v)", cause, err)
	}
	return cause
}

// upsertProceedings inserts newly discovered proceedings (computing their
// permanent folder name) and refreshes bookkeeping fields on existing ones.
// It returns each proceeding's folder name (existing or newly created) for
// use by the download phase.
func (s *Syncer) upsertProceedings(discovered []fmcsite.DiscoveredProceeding) (map[string]string, int, error) {
	folderNames := make(map[string]string, len(discovered))
	newCount := 0

	for _, d := range discovered {
		existing, ok, err := s.Store.GetProceeding(d.Number)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			folder := ProceedingFolderName(d.Number, d.Title)
			if err := s.Store.InsertProceeding(store.Proceeding{
				ProceedingNumber: d.Number,
				Title:            d.Title,
				FolderName:       folder,
				CreatedDate:      d.CreatedDate,
				ProceedingType:   string(d.Type),
				IsClosed:         d.IsClosed,
			}); err != nil {
				return nil, 0, err
			}
			folderNames[d.Number] = folder
			newCount++
			continue
		}
		if err := s.Store.UpdateProceedingMetadata(d.Number, d.Title, d.CreatedDate, string(d.Type), d.IsClosed); err != nil {
			return nil, 0, err
		}
		folderNames[d.Number] = existing.FolderName
	}
	return folderNames, newCount, nil
}

// syncAllProceedings is the default, full-fidelity document discovery
// strategy: every discovered proceeding's detail page is fetched and
// diffed. See syncSince for the --since alternative.
func (s *Syncer) syncAllProceedings(ctx context.Context, discovered []fmcsite.DiscoveredProceeding, folderNames map[string]string, c *counters) error {
	numbers := make([]string, 0, len(discovered))
	for _, d := range discovered {
		numbers = append(numbers, d.Number)
	}
	if s.Opts.Limit > 0 && len(numbers) > s.Opts.Limit {
		numbers = numbers[:s.Opts.Limit]
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.Opts.Concurrency)
	for _, number := range numbers {
		number := number
		folder := folderNames[number]
		g.Go(func() error {
			c.scanned.Add(1)
			if err := s.syncProceeding(gctx, number, folder, c); err != nil {
				s.Opts.progress("proceeding %s: %v", number, err)
			}
			return nil // one proceeding's failure never aborts the whole run
		})
	}
	_ = g.Wait()
	return nil
}

// syncProceeding fetches one proceeding's current document list and
// downloads whatever isn't already recorded in the store.
func (s *Syncer) syncProceeding(ctx context.Context, number, folderName string, c *counters) error {
	detail, err := s.Client.FetchProceedingDetail(ctx, number)
	if err != nil {
		c.failed.Add(1)
		return err
	}

	existing, err := s.Store.ExistingDocumentNumbers(number)
	if err != nil {
		return err
	}

	folderPath := filepath.Join(s.Opts.ProceedingsDir, folderName)
	for _, doc := range detail.Documents {
		if existing[doc.Number] {
			continue
		}
		// The site's own document listing has occasionally repeated a
		// document number within one proceeding; without this, the second
		// occurrence would re-download and overwrite the first before
		// failing to insert (unique_key already taken).
		existing[doc.Number] = true

		s.downloadOne(ctx, number, folderPath, doc, c)
	}

	if !s.Opts.DryRun {
		if err := s.Store.UpdateProceedingSyncState(number, detail.LastUpdated, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

// syncSince finds documents via a single site-wide DocumentSearch filtered
// by serve date, rather than fetching every proceeding's detail page. Every
// proceeding referenced by a result row is guaranteed to already be in
// folderNames, because the discovery pass in Run always runs first and
// covers every proceeding on the site, regardless of Since.
func (s *Syncer) syncSince(ctx context.Context, since time.Time, folderNames map[string]string, c *counters) error {
	rows, err := s.Client.SearchDocumentsSince(ctx, since)
	if err != nil {
		return err
	}
	s.Opts.progress("document search since %s found %d documents", since.Format("2006-01-02"), len(rows))

	if s.Opts.Limit > 0 && len(rows) > s.Opts.Limit {
		rows = rows[:s.Opts.Limit]
	}

	type touched struct {
		proceedingNumber string
		servedDate       time.Time
	}
	results := make(chan touched, len(rows))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.Opts.Concurrency)
	for _, row := range rows {
		g.Go(func() error {
			exists, err := s.Store.DocumentExists(row.ProceedingNumber, row.Number)
			if err != nil {
				c.failed.Add(1)
				s.Opts.progress("failed to check %s doc %d: %v", row.ProceedingNumber, row.Number, err)
				return nil
			}
			if exists {
				return nil
			}

			folder, ok := folderNames[row.ProceedingNumber]
			if !ok {
				// Shouldn't happen: Run's discovery pass covers every
				// proceeding on the site before this ever runs.
				c.failed.Add(1)
				s.Opts.progress("skipping %s doc %d: proceeding wasn't in this run's discovery pass", row.ProceedingNumber, row.Number)
				return nil
			}

			doc := fmcsite.DocumentRow{
				Number:      row.Number,
				ServedDate:  row.ServedDate,
				Description: row.Description,
				SourceURL:   row.SourceURL,
			}
			folderPath := filepath.Join(s.Opts.ProceedingsDir, folder)
			if s.downloadOne(gctx, row.ProceedingNumber, folderPath, doc, c) {
				results <- touched{row.ProceedingNumber, row.ServedDate}
			}
			return nil
		})
	}
	_ = g.Wait()
	close(results)

	// last_updated is normally the site's own "Last Updated" label (from a
	// proceeding's detail page); in this mode we only have the documents we
	// actually saw, so the latest served date among them is the closest
	// available proxy.
	maxServedByProceeding := make(map[string]time.Time)
	for r := range results {
		if cur, ok := maxServedByProceeding[r.proceedingNumber]; !ok || r.servedDate.After(cur) {
			maxServedByProceeding[r.proceedingNumber] = r.servedDate
		}
	}

	if !s.Opts.DryRun {
		now := time.Now()
		for number, maxServed := range maxServedByProceeding {
			if err := s.Store.UpdateProceedingSyncState(number, maxServed, now); err != nil {
				return err
			}
		}
	}
	c.scanned.Add(int64(len(maxServedByProceeding)))
	return nil
}

// downloadOne handles one not-yet-seen document: downloads it (or records
// it as unavailable on a 404), updating c accordingly. It reports whether
// the proceeding should be considered "touched" (downloaded or recorded as
// unavailable) for last_updated bookkeeping purposes.
func (s *Syncer) downloadOne(ctx context.Context, proceedingNumber, folderPath string, doc fmcsite.DocumentRow, c *counters) (touched bool) {
	c.newDocuments.Add(1)

	if s.Opts.DryRun {
		s.Opts.progress("[dry-run] would download %s doc %d: %s", proceedingNumber, doc.Number, doc.Description)
		return true
	}

	record, err := downloadDocument(ctx, s.Client, proceedingNumber, folderPath, doc)
	if err != nil {
		if errors.Is(err, fmcsite.ErrNotFound) {
			if err := s.Store.InsertDocument(unavailableDocument(proceedingNumber, doc)); err != nil {
				c.failed.Add(1)
				s.Opts.progress("failed to record %s doc %d as unavailable: %v", proceedingNumber, doc.Number, err)
				return false
			}
			c.unavailable.Add(1)
			s.Opts.progress("%s doc %d: 404, recorded as unavailable (won't retry automatically)", proceedingNumber, doc.Number)
			return true
		}
		c.failed.Add(1)
		s.Opts.progress("failed to download %s doc %d: %v", proceedingNumber, doc.Number, err)
		return false
	}
	if err := s.Store.InsertDocument(record); err != nil {
		c.failed.Add(1)
		s.Opts.progress("failed to record %s doc %d: %v", proceedingNumber, doc.Number, err)
		return false
	}
	c.downloaded.Add(1)
	return true
}
