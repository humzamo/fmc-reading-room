package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

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

	client, err := fmcsite.NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.RequestDelay = 0

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

	var newDocs, downloaded, unavailable, failed atomic.Int64
	if err := syncer.syncProceeding(context.Background(), "26-12", "26-12_Test Proceeding", &newDocs, &downloaded, &unavailable, &failed); err != nil {
		t.Fatalf("syncProceeding: %v", err)
	}

	if got := unavailable.Load(); got != 1 {
		t.Errorf("unavailable = %d, want 1", got)
	}
	if got := downloaded.Load(); got != 0 {
		t.Errorf("downloaded = %d, want 0", got)
	}
	if got := failed.Load(); got != 0 {
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

	var newDocs, downloaded, unavailable, failed atomic.Int64
	if err := syncer.syncProceeding(context.Background(), "26-12", "26-12_Test Proceeding", &newDocs, &downloaded, &unavailable, &failed); err != nil {
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

	var newDocs2, downloaded2, unavailable2, failed2 atomic.Int64
	if err := syncer.syncProceeding(context.Background(), "26-12", "26-12_Test Proceeding", &newDocs2, &downloaded2, &unavailable2, &failed2); err != nil {
		t.Fatalf("second syncProceeding: %v", err)
	}
	if got := newDocs2.Load(); got != 0 {
		t.Errorf("second pass newDocuments = %d, want 0 (already recorded)", got)
	}
	if got := unavailable2.Load(); got != 0 {
		t.Errorf("second pass unavailable = %d, want 0 (already recorded, not re-flagged)", got)
	}
}
