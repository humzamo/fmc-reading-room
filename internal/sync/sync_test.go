package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/humzamoazzam/fmc-reading-room/internal/fmcsite"
	"github.com/humzamoazzam/fmc-reading-room/internal/store"
)

// proceedingDetailFixture is a minimal but structurally faithful stand-in
// for a real proceeding detail page: one document row whose DocumentId
// (the last hidden column) is "999", matched to the 404 route registered
// in the test server below.
const proceedingDetailFixture = `<html><body>
<span id="MainContent_lblTitle">26-12 - Test Proceeding</span>
<span id="MainContent_lblLastUpdated">Last Updated: 09/09/2026</span>
<table class="rgMasterTable"><tbody>
<tr class="rgRow">
<td>1</td><td>08/24/2026</td><td>A Document That 404s</td><td>x</td><td>.pdf</td><td>x</td><td>x</td><td>x</td><td>999</td>
</tr>
</tbody></table>
</body></html>`

func newTestSyncer(t *testing.T) (*Syncer, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/proceeding/26-12/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(proceedingDetailFixture))
	})
	mux.HandleFunc("/documents/999", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := fmcsite.NewClient(server.URL, 0)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.InsertProceeding(store.Proceeding{
		ProceedingNumber: "26-12",
		Title:            "Test Proceeding",
		FolderName:       "26-12_Test Proceeding",
	}); err != nil {
		t.Fatalf("InsertProceeding: %v", err)
	}

	return New(client, st, Options{ProceedingsDir: t.TempDir()}), server
}

func TestSyncProceedingRecordsNotFoundAsUnavailable(t *testing.T) {
	syncer, _ := newTestSyncer(t)

	var c counters
	if err := syncer.syncProceeding(context.Background(), "26-12", "26-12_Test Proceeding", &c); err != nil {
		t.Fatalf("syncProceeding: %v", err)
	}

	if got := c.unavailable.Load(); got != 1 {
		t.Errorf("unavailable = %d, want 1", got)
	}
	if got := c.downloaded.Load(); got != 0 {
		t.Errorf("downloaded = %d, want 0", got)
	}
	if got := c.failed.Load(); got != 0 {
		t.Errorf("failed = %d, want 0 (a 404 is recorded, not a failure)", got)
	}

	existing, err := syncer.Store.ExistingDocumentNumbers("26-12")
	if err != nil {
		t.Fatalf("ExistingDocumentNumbers: %v", err)
	}
	if !existing[1] {
		t.Fatalf("expected document 1 to be recorded even though its file 404s")
	}
}

func TestSyncProceedingDoesNotRetryUnavailableDocuments(t *testing.T) {
	syncer, server := newTestSyncer(t)

	var c counters
	if err := syncer.syncProceeding(context.Background(), "26-12", "26-12_Test Proceeding", &c); err != nil {
		t.Fatalf("first syncProceeding: %v", err)
	}

	// If a second sync pass tried the dead link again, it would hit the
	// same 404 route; the point of this test is that it shouldn't even
	// try, because the document is already recorded.
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request on second pass: %s", r.URL.Path)
	})
	// The proceeding detail page itself is still fetched every run (that's
	// how new documents get discovered), so restore just that route.
	mux := http.NewServeMux()
	mux.HandleFunc("/proceeding/26-12/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(proceedingDetailFixture))
	})
	mux.HandleFunc("/documents/999", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("document 999 should not be re-fetched once recorded as unavailable")
	})
	server.Config.Handler = mux

	var c2 counters
	if err := syncer.syncProceeding(context.Background(), "26-12", "26-12_Test Proceeding", &c2); err != nil {
		t.Fatalf("second syncProceeding: %v", err)
	}
	if got := c2.newDocuments.Load(); got != 0 {
		t.Errorf("second pass newDocuments = %d, want 0 (already recorded)", got)
	}
	if got := c2.unavailable.Load(); got != 0 {
		t.Errorf("second pass unavailable = %d, want 0 (already recorded, not re-flagged)", got)
	}
}

// documentSearchFixture returns one DocumentSearch result row for
// proceeding "89-01" (deliberately NOT "26-12", to prove the found document
// doesn't depend on which proceeding it's attached to — only on its own
// serve date) whose DocumentId is "555".
const documentSearchFixture = `<html><body><form>
<table class="rgMasterTable"><tbody>
<tr class="rgRow">
<td><a href="x">89-01</a></td><td>3</td><td>09/01/2026</td><td>combined title</td><td>A Document From An Old Proceeding</td><td>.pdf</td><td>x</td><td>89-01</td><td><a href="documents/555">x</a></td><td>x</td><td>555</td><td>1</td>
</tr>
</tbody></table>
</form></body></html>`

func TestSyncSinceFindsNewDocumentOnOldProceeding(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/DocumentSearch", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(documentSearchFixture))
	})
	mux.HandleFunc("/documents/555", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-fake"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := fmcsite.NewClient(server.URL, 0)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// "89-01" simulates an old, already-known proceeding (created long
	// before the --since cutoff) that just received a brand-new filing.
	if err := st.InsertProceeding(store.Proceeding{
		ProceedingNumber: "89-01",
		Title:            "An Old Proceeding",
		FolderName:       "89-01_An Old Proceeding",
		CreatedDate:      mustParseDate2006(t, "1989-01-01"),
	}); err != nil {
		t.Fatalf("InsertProceeding: %v", err)
	}

	syncer := New(client, st, Options{ProceedingsDir: t.TempDir()})
	folderNames := map[string]string{"89-01": "89-01_An Old Proceeding"}

	var c counters
	if err := syncer.syncSince(context.Background(), mustParseDate2006(t, "2026-01-01"), folderNames, &c); err != nil {
		t.Fatalf("syncSince: %v", err)
	}

	if got := c.downloaded.Load(); got != 1 {
		t.Fatalf("downloaded = %d, want 1 (the old proceeding's new document must still be found)", got)
	}

	exists, err := st.DocumentExists("89-01", 3)
	if err != nil {
		t.Fatalf("DocumentExists: %v", err)
	}
	if !exists {
		t.Errorf("expected document 89-01_3 to be recorded")
	}
}

func mustParseDate2006(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parsing date %q: %v", s, err)
	}
	return parsed
}
