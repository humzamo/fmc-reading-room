package sync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/humzamoazzam/fmc-reading-room/internal/fmcsite"
)

// TestDownloadDocumentRejectsFakeSuccess covers a real bug found in this
// site's data: broken links return HTTP 200 with a small HTML/text error
// body (sometimes even with a Content-Type header claiming application/pdf)
// instead of an honest 404. downloadDocument must treat these the same as
// a real 404 rather than saving the error body as if it were the document.
func TestDownloadDocumentRejectsFakeSuccess(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
	}{
		{"html error page, honest content-type", "text/html; charset=utf-8", "File not found!"},
		{"app error page", "text/html; charset=utf-8", "ERROR: There is no row at position 0."},
		{"empty body", "application/pdf; charset=utf-8", ""},
		{"html 404 page, lying content-type", "application/pdf; charset=utf-8", "<html><head><title>Error 404</title></head><body>404 Not Found</body></html>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client, err := fmcsite.NewClient(server.URL, 0)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			doc := fmcsite.DocumentRow{Number: 1, Description: "Test Document", SourceURL: server.URL + "/documents/1"}
			_, err = downloadDocument(context.Background(), client, "26-12", t.TempDir(), doc)
			if !errors.Is(err, fmcsite.ErrNotFound) {
				t.Fatalf("err = %v, want fmcsite.ErrNotFound", err)
			}
		})
	}
}

func TestDownloadDocumentAcceptsRealPDF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf; charset=utf-8")
		w.Write([]byte("%PDF-1.4\n%fake but valid enough PDF header for this test\n"))
	}))
	defer server.Close()

	client, err := fmcsite.NewClient(server.URL, 0)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	doc := fmcsite.DocumentRow{Number: 1, Description: "Test Document", SourceURL: server.URL + "/documents/1"}
	record, err := downloadDocument(context.Background(), client, "26-12", t.TempDir(), doc)
	if err != nil {
		t.Fatalf("downloadDocument: %v", err)
	}
	if record.FileName == "" {
		t.Errorf("expected a FileName to be set for a real document")
	}
}
