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

// downloadDocument fetches doc into folderPath, naming it only once the
// response reveals a content type. Writes to a temp file and renames into
// place, so a crash never leaves a half-written file under its real name.
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

// unavailableDocument builds a placeholder row for a document whose file
// 404s: the site's own fields are kept, but FileName/FilePath are empty and
// SourceURL is replaced with the store.UnavailableSourceURL sentinel.
func unavailableDocument(proceedingNumber string, doc fmcsite.DocumentRow) store.Document {
	return store.Document{
		UniqueKey:        store.UniqueKey(proceedingNumber, doc.Number),
		ProceedingNumber: proceedingNumber,
		DocumentNumber:   doc.Number,
		ServedDate:       doc.ServedDate,
		Description:      doc.Description,
		SourceURL:        store.UnavailableSourceURL,
		DownloadedAt:     time.Now(),
	}
}
