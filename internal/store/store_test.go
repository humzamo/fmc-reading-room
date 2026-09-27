package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProceedingInsertUpdatePreservesFolderName(t *testing.T) {
	s := openTestStore(t)

	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	err := s.InsertProceeding(Proceeding{
		ProceedingNumber: "26-01",
		Title:            "Original Title",
		FolderName:       "26-01_Original Title",
		CreatedDate:      created,
		ProceedingType:   string("Dockets"),
	})
	if err != nil {
		t.Fatalf("InsertProceeding: %v", err)
	}

	// A later crawl finds a changed title; folder name must not move.
	if err := s.UpdateProceedingMetadata("26-01", "Changed Title", created, "Dockets", true); err != nil {
		t.Fatalf("UpdateProceedingMetadata: %v", err)
	}

	got, ok, err := s.GetProceeding("26-01")
	if err != nil || !ok {
		t.Fatalf("GetProceeding: ok=%v err=%v", ok, err)
	}
	if got.FolderName != "26-01_Original Title" {
		t.Errorf("FolderName = %q, want unchanged", got.FolderName)
	}
	if got.Title != "Changed Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Changed Title")
	}
	if !got.IsClosed {
		t.Errorf("IsClosed = false, want true")
	}
}

func TestDocumentDiffByNumber(t *testing.T) {
	s := openTestStore(t)
	if err := s.InsertProceeding(Proceeding{ProceedingNumber: "26-01", Title: "T", FolderName: "26-01_T"}); err != nil {
		t.Fatalf("InsertProceeding: %v", err)
	}
	if err := s.InsertDocument(Document{
		UniqueKey:        UniqueKey("26-01", 1),
		ProceedingNumber: "26-01",
		DocumentNumber:   1,
		Description:      "First",
		SourceURL:        "https://example/documents/1",
		FileName:         "01_First.pdf",
		FilePath:         "proceedings/26-01_T/01_First.pdf",
		DownloadedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	existing, err := s.ExistingDocumentNumbers("26-01")
	if err != nil {
		t.Fatalf("ExistingDocumentNumbers: %v", err)
	}
	if !existing[1] {
		t.Errorf("expected document 1 to be present")
	}
	if existing[2] {
		t.Errorf("expected document 2 to be absent")
	}
}

func TestSyncRunLifecycle(t *testing.T) {
	s := openTestStore(t)

	id, err := s.StartRun(time.Now())
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.FinishRun(id, time.Now(), RunStatusSuccess, 10, 1, 2, 2, 0, ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	run, ok, err := s.LatestRun()
	if err != nil || !ok {
		t.Fatalf("LatestRun: ok=%v err=%v", ok, err)
	}
	if run.Status != RunStatusSuccess {
		t.Errorf("Status = %q, want %q", run.Status, RunStatusSuccess)
	}
	if run.NewDocuments != 2 {
		t.Errorf("NewDocuments = %d, want 2", run.NewDocuments)
	}
}
