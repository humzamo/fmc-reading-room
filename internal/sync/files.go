package sync

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxNameLength keeps sanitized names comfortably under filesystem limits
// (macOS/Linux allow 255 bytes per path component) even after a suffix like
// "_Description.pdf" is appended.
const maxNameLength = 150

var illegalChars = regexp.MustCompile(`[/\\:*?"<>|]`)
var whitespaceRun = regexp.MustCompile(`\s+`)

// sanitizeName makes s safe to use as a single path component.
func sanitizeName(s string) string {
	s = illegalChars.ReplaceAllString(s, "-")
	s = whitespaceRun.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		s = "untitled"
	}
	if len(s) > maxNameLength {
		s = strings.TrimSpace(s[:maxNameLength])
	}
	return s
}

// ProceedingFolderName builds the on-disk folder name for a proceeding.
func ProceedingFolderName(number, title string) string {
	return fmt.Sprintf("%s_%s", sanitizeName(number), sanitizeName(title))
}

// DocumentFileName builds the on-disk file name for a document.
func DocumentFileName(documentNumber int, description, ext string) string {
	return fmt.Sprintf("%02d_%s%s", documentNumber, sanitizeName(description), ext)
}

// extensionFor picks a file extension for a downloaded document, preferring
// the Content-Type header, then the original filename the server suggested,
// and finally defaulting to .pdf since that's effectively every document on
// this site.
func extensionFor(contentType, originalFileName string) string {
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
				return exts[0]
			}
		}
	}
	if ext := filepath.Ext(originalFileName); ext != "" {
		return ext
	}
	return ".pdf"
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("sync: creating directory %s: %w", path, err)
	}
	return nil
}
