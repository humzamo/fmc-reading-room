// Package sync orchestrates one full pass over the FMC reading room: it
// discovers every proceeding, upserts its bookkeeping fields, then fetches
// each proceeding's document list and downloads whatever isn't already on
// disk. See the project plan for why this is idempotent and diff-based
// rather than date-filtered.
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
	// Limit caps how many proceedings get their documents fetched/
	// downloaded, for smoke-testing against the live site without doing a
	// full crawl. Proceeding discovery/metadata is unaffected. 0 = no limit.
	Limit int
	// DryRun discovers and diffs normally but skips writing files and
	// documents rows, only logging what would happen.
	DryRun bool
	// Concurrency bounds how many proceedings are processed at once.
	// Defaults to 5.
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
	return &Syncer{Client: client, Store: st, Opts: opts}
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

	numbers := make([]string, 0, len(discovered))
	for _, d := range discovered {
		numbers = append(numbers, d.Number)
	}
	if s.Opts.Limit > 0 && len(numbers) > s.Opts.Limit {
		numbers = numbers[:s.Opts.Limit]
	}

	var scanned, newDocuments, downloaded, unavailable, failed atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.Opts.Concurrency)
	for _, number := range numbers {
		number := number
		folder := folderNames[number]
		g.Go(func() error {
			scanned.Add(1)
			if err := s.syncProceeding(gctx, number, folder, &newDocuments, &downloaded, &unavailable, &failed); err != nil {
				s.Opts.progress("proceeding %s: %v", number, err)
			}
			return nil // one proceeding's failure never aborts the whole run
		})
	}
	_ = g.Wait()

	finishedAt := time.Now()
	run := store.SyncRun{
		ID:                   runID,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		Status:               store.RunStatusSuccess,
		ProceedingsScanned:   int(scanned.Load()),
		NewProceedings:       newProceedings,
		NewDocuments:         int(newDocuments.Load()),
		DocumentsDownloaded:  int(downloaded.Load()),
		DocumentsUnavailable: int(unavailable.Load()),
		DocumentsFailed:      int(failed.Load()),
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

// syncProceeding fetches one proceeding's current document list and
// downloads whatever isn't already recorded in the store.
func (s *Syncer) syncProceeding(ctx context.Context, number, folderName string, newDocuments, downloaded, unavailable, failed *atomic.Int64) error {
	detail, err := s.Client.FetchProceedingDetail(ctx, number)
	if err != nil {
		failed.Add(1)
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
		newDocuments.Add(1)

		if s.Opts.DryRun {
			s.Opts.progress("[dry-run] would download %s doc %d: %s", number, doc.Number, doc.Description)
			continue
		}

		record, err := downloadDocument(ctx, s.Client, number, folderPath, doc)
		if err != nil {
			if errors.Is(err, fmcsite.ErrNotFound) {
				if err := s.Store.InsertDocument(unavailableDocument(number, doc)); err != nil {
					failed.Add(1)
					s.Opts.progress("failed to record %s doc %d as unavailable: %v", number, doc.Number, err)
					continue
				}
				unavailable.Add(1)
				s.Opts.progress("%s doc %d: 404, recorded as unavailable (won't retry automatically)", number, doc.Number)
				continue
			}
			failed.Add(1)
			s.Opts.progress("failed to download %s doc %d: %v", number, doc.Number, err)
			continue
		}
		if err := s.Store.InsertDocument(record); err != nil {
			failed.Add(1)
			s.Opts.progress("failed to record %s doc %d: %v", number, doc.Number, err)
			continue
		}
		downloaded.Add(1)
	}

	if !s.Opts.DryRun {
		if err := s.Store.UpdateProceedingSyncState(number, detail.LastUpdated, time.Now()); err != nil {
			return err
		}
	}
	return nil
}
