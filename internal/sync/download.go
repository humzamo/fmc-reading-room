package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/humzamoazzam/fmc-reading-room/internal/fmcsite"
	"github.com/humzamoazzam/fmc-reading-room/internal/store"
)

// downloadDocument fetches doc into folderPath, choosing its final file
// name only after the response headers reveal a content type (so the
// extension is right). It writes to a temp file first and renames into
// place, so a crash or network failure never leaves a half-written file
// under its real name.
func downloadDocument(ctx context.Context, client *fmcsite.Client, proceedingNumber, folderPath string, doc fmcsite.DocumentRow) (store.Document, error) {
	if err := ensureDir(folderPath); err != nil {
		return store.Document{}, err
	}

	tmp, err := os.CreateTemp(folderPath, fmt.Sprintf(".tmp-doc%d-*", doc.Number))
	if err != nil {
		return store.Document{}, fmt.Errorf("sync: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once successfully renamed away

	result, err := client.Download(ctx, doc.SourceURL, tmp)
	closeErr := tmp.Close()
	if err != nil {
		return store.Document{}, fmt.Errorf("sync: downloading document %d for %s: %w", doc.Number, proceedingNumber, err)
	}
	if closeErr != nil {
		return store.Document{}, fmt.Errorf("sync: closing temp file for document %d: %w", doc.Number, closeErr)
	}

	ext := extensionFor(result.ContentType, result.FileName)
	fileName := DocumentFileName(doc.Number, doc.Description, ext)
	finalPath := filepath.Join(folderPath, fileName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return store.Document{}, fmt.Errorf("sync: saving document %d for %s: %w", doc.Number, proceedingNumber, err)
	}

	return store.Document{
		UniqueKey:        store.UniqueKey(proceedingNumber, doc.Number),
		ProceedingNumber: proceedingNumber,
		DocumentNumber:   doc.Number,
		ServedDate:       doc.ServedDate,
		Description:      doc.Description,
		SourceURL:        doc.SourceURL,
		FileName:         fileName,
		FilePath:         filepath.Join(filepath.Base(folderPath), fileName),
		ContentType:      result.ContentType,
		FileSize:         result.Size,
		DownloadedAt:     time.Now(),
	}, nil
}
