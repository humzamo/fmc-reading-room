// Command fmc-reading-room mirrors the FMC reading room
// (https://www2.fmc.gov/readingroom/) to local disk + SQLite. Run `sync`
// periodically (via cron/launchd, or by hand) to download whatever's new;
// run `status` to see when it last ran and what happened.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/humzamoazzam/fmc-reading-room/internal/fmcsite"
	"github.com/humzamoazzam/fmc-reading-room/internal/store"
	"github.com/humzamoazzam/fmc-reading-room/internal/sync"
)

const defaultBaseURL = "https://www2.fmc.gov/readingroom"

func main() {
	// The reading room's server occasionally sends a stray byte on a
	// keep-alive connection Go's transport already considers idle; this is
	// harmless (every request still succeeds), but net/http logs it via
	// the standard logger. Filter just that one diagnostic line rather
	// than disabling keep-alives, which would meaningfully slow down a
	// ~700-request discovery pass for no benefit.
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

type globalFlags struct {
	dbPath         string
	proceedingsDir string
	baseURL        string
	concurrency    int
	rateLimitMS    int
}

func newRootCmd() *cobra.Command {
	flags := &globalFlags{}

	root := &cobra.Command{
		Use:   "fmc-reading-room",
		Short: "Mirror the FMC reading room to local disk + SQLite",
	}
	root.PersistentFlags().StringVar(&flags.dbPath, "db", "fmc.db", "path to the SQLite database")
	root.PersistentFlags().StringVar(&flags.proceedingsDir, "proceedings-dir", "proceedings", "path to the proceedings download folder")
	root.PersistentFlags().StringVar(&flags.baseURL, "base-url", defaultBaseURL, "reading room base URL")
	root.PersistentFlags().IntVar(&flags.concurrency, "concurrency", 5, "number of proceedings processed concurrently")
	root.PersistentFlags().IntVar(&flags.rateLimitMS, "rate-limit-ms", 250, "delay before each outgoing request, in milliseconds")

	root.AddCommand(newSyncCmd(flags))
	root.AddCommand(newStatusCmd(flags))
	return root
}

func (f *globalFlags) newClient() (*fmcsite.Client, error) {
	client, err := fmcsite.NewClient(f.baseURL)
	if err != nil {
		return nil, err
	}
	client.RequestDelay = time.Duration(f.rateLimitMS) * time.Millisecond
	return client, nil
}

func newSyncCmd(flags *globalFlags) *cobra.Command {
	var limit int
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download every document that isn't already saved",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := flags.newClient()
			if err != nil {
				return err
			}
			st, err := store.Open(flags.dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			syncer := sync.New(client, st, sync.Options{
				ProceedingsDir: flags.proceedingsDir,
				Limit:          limit,
				DryRun:         dryRun,
				Concurrency:    flags.concurrency,
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
	cmd.Flags().IntVar(&limit, "limit", 0, "only fetch documents for the first N discovered proceedings (0 = no limit)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "discover and diff normally, but don't write files or DB rows")
	return cmd
}

func newStatusCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the last sync run and current totals",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(flags.dbPath)
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
