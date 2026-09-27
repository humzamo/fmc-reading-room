// Command fmc-reading-room mirrors the FMC reading room
// (https://www2.fmc.gov/readingroom/) to local disk + SQLite. Run `sync`
// periodically (via cron/launchd, or by hand) to download whatever's new;
// run `status` to see when it last ran and what happened. See README.md.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/humzamoazzam/fmc-reading-room/internal/fmcsite"
	"github.com/humzamoazzam/fmc-reading-room/internal/store"
	"github.com/humzamoazzam/fmc-reading-room/internal/sync"
)

// Only --dry-run and --since are worth changing at runtime; everything
// else is fixed here — edit and rebuild for a different value.
const (
	baseURL        = "https://www2.fmc.gov/readingroom"
	dbPath         = "fmc.db"
	proceedingsDir = "proceedings"

	// Tuned empirically against the live site (see README.md).
	concurrency = 20
	rateLimitMS = 75

	// limit caps proceedings/search-rows processed; for dev smoke-testing
	// only. 0 = no limit.
	limit = 0

	// exportPath is where the search UI's data file is written.
	exportPath = "docs/data/documents.json"
)

func main() {
	// The server occasionally sends a stray byte on an idle keep-alive
	// connection; harmless, but net/http logs it. Filter that one line
	// rather than disabling keep-alives (which would slow discovery down).
	log.SetOutput(dropLines(os.Stderr, "Unsolicited response received on idle HTTP channel"))

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

type lineFilterWriter struct {
	dst  io.Writer
	drop string
}

func dropLines(dst io.Writer, drop string) io.Writer {
	return lineFilterWriter{dst: dst, drop: drop}
}

func (w lineFilterWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), w.drop) {
		return len(p), nil
	}
	return w.dst.Write(p)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "fmc-reading-room",
		Short: "Mirror the FMC reading room to local disk + SQLite",
	}
	root.AddCommand(newSyncCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newVerifyCmd())
	return root
}

func newClient() (*fmcsite.Client, error) {
	client, err := fmcsite.NewClient(baseURL, rateLimitMS)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func newSyncCmd() *cobra.Command {
	var dryRun bool
	var since string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download every document that isn't already saved",
		RunE: func(cmd *cobra.Command, args []string) error {
			var sinceDate time.Time
			if since != "" {
				parsed, err := time.Parse("2006-01-02", since)
				if err != nil {
					return fmt.Errorf("--since must be YYYY-MM-DD: %w", err)
				}
				sinceDate = parsed
			}

			client, err := newClient()
			if err != nil {
				return err
			}
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			syncer := sync.New(client, st, sync.Options{
				ProceedingsDir: proceedingsDir,
				Since:          sinceDate,
				Limit:          limit,
				DryRun:         dryRun,
				Concurrency:    concurrency,
				Progress: func(format string, args ...any) {
					log.Printf(format, args...)
				},
			})

			run, err := syncer.Run(context.Background())
			if err != nil {
				return err
			}
			printRun(run)
			if run.Status != store.RunStatusSuccess {
				return fmt.Errorf("sync run %d failed: %s", run.ID, run.Error)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "discover and diff normally, but don't write files or DB rows")
	cmd.Flags().StringVar(&since, "since", "", "YYYY-MM-DD: only fetch documents served on or after this date (see README.md)")
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the last sync run and current totals",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			run, ok, err := st.LatestRun()
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("No sync has run yet.")
			} else {
				printRun(run)
			}

			proceedings, err := st.CountProceedings()
			if err != nil {
				return err
			}
			documents, err := st.CountDocuments()
			if err != nil {
				return err
			}
			fmt.Printf("Total: %d proceedings, %d documents on disk.\n", proceedings, documents)
			return nil
		},
	}
}

// exportDocument is the search UI's per-document JSON shape.
type exportDocument struct {
	ProceedingNumber string `json:"proceeding_number"`
	ProceedingTitle  string `json:"proceeding_title"`
	DocumentNumber   int    `json:"document_number"`
	Description      string `json:"description"`
	ServedDate       string `json:"served_date,omitempty"` // YYYY-MM-DD
	FileName         string `json:"file_name,omitempty"`
	FilePath         string `json:"file_path,omitempty"` // relative to proceedings/, for linking to the raw file in the repo
	Unavailable      bool   `json:"unavailable,omitempty"`
}

func newExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Write the search UI's data file (docs/data/documents.json)",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			rows, err := st.ListDocumentsForExport()
			if err != nil {
				return err
			}

			out := make([]exportDocument, len(rows))
			for i, r := range rows {
				out[i] = exportDocument{
					ProceedingNumber: r.ProceedingNumber,
					ProceedingTitle:  r.ProceedingTitle,
					DocumentNumber:   r.DocumentNumber,
					Description:      r.Description,
					FileName:         r.FileName,
					FilePath:         r.FilePath,
					Unavailable:      r.Unavailable,
				}
				if !r.ServedDate.IsZero() {
					out[i].ServedDate = r.ServedDate.Format("2006-01-02")
				}
			}

			if err := os.MkdirAll(filepath.Dir(exportPath), 0o755); err != nil {
				return err
			}
			data, err := json.Marshal(out)
			if err != nil {
				return err
			}
			if err := os.WriteFile(exportPath, data, 0o644); err != nil {
				return err
			}
			fmt.Printf("Wrote %d documents to %s\n", len(out), exportPath)
			return nil
		},
	}
}

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Check every downloaded file is a real PDF, fixing up any that aren't",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			rows, err := st.ListDocumentsForExport()
			if err != nil {
				return err
			}

			checked, fixed := 0, 0
			for _, r := range rows {
				if r.Unavailable || r.FilePath == "" {
					continue
				}
				checked++
				fullPath := filepath.Join(proceedingsDir, r.FilePath)
				valid, err := sync.IsValidPDF(fullPath)
				if err != nil {
					fmt.Printf("could not check %s doc %d (%s): %v\n", r.ProceedingNumber, r.DocumentNumber, fullPath, err)
					continue
				}
				if valid {
					continue
				}

				uniqueKey := store.UniqueKey(r.ProceedingNumber, r.DocumentNumber)
				if err := os.Remove(fullPath); err != nil {
					fmt.Printf("failed to remove bad file for %s: %v\n", uniqueKey, err)
					continue
				}
				if err := st.MarkDocumentUnavailable(uniqueKey); err != nil {
					fmt.Printf("failed to mark %s unavailable: %v\n", uniqueKey, err)
					continue
				}
				fixed++
				fmt.Printf("fixed: %s was not a real PDF, marked unavailable\n", uniqueKey)
			}

			fmt.Printf("\nChecked %d files, fixed %d.\n", checked, fixed)
			return nil
		},
	}
}

func printRun(run store.SyncRun) {
	fmt.Printf("Run #%d: %s\n", run.ID, run.Status)
	fmt.Printf("  started:  %s\n", run.StartedAt.Format(time.RFC1123))
	if !run.FinishedAt.IsZero() {
		fmt.Printf("  finished: %s (took %s)\n", run.FinishedAt.Format(time.RFC1123), run.FinishedAt.Sub(run.StartedAt).Round(time.Second))
	}
	fmt.Printf("  proceedings scanned: %d (new: %d)\n", run.ProceedingsScanned, run.NewProceedings)
	fmt.Printf("  documents: %d new, %d downloaded, %d unavailable (404, recorded), %d failed\n",
		run.NewDocuments, run.DocumentsDownloaded, run.DocumentsUnavailable, run.DocumentsFailed)
	if run.Error != "" {
		fmt.Printf("  error: %s\n", run.Error)
	}
}
