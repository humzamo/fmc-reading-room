package sync

import (
	"strings"
	"testing"
)

func TestSanitizeNameStripsIllegalCharacters(t *testing.T) {
	got := sanitizeName(`Down Quark/Systems: "Weird" <name> | v. Zim*Line?`)
	for _, bad := range []string{"/", ":", `"`, "<", ">", "|", "*", "?"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizeName result %q still contains %q", got, bad)
		}
	}
}

func TestSanitizeNameTruncatesLongInput(t *testing.T) {
	long := strings.Repeat("a", 500)
	got := sanitizeName(long)
	if len(got) > maxNameLength {
		t.Errorf("len(got) = %d, want <= %d", len(got), maxNameLength)
	}
}

func TestSanitizeNameHandlesEmpty(t *testing.T) {
	if got := sanitizeName("   "); got != "untitled" {
		t.Errorf("sanitizeName(blank) = %q, want %q", got, "untitled")
	}
}

func TestProceedingFolderName(t *testing.T) {
	got := ProceedingFolderName("26-12", "Samsung Electronics America, Inc. v. CMA CGM S.A.")
	want := "26-12_Samsung Electronics America, Inc. v. CMA CGM S.A."
	if got != want {
		t.Errorf("ProceedingFolderName = %q, want %q", got, want)
	}
}

func TestDocumentFileName(t *testing.T) {
	got := DocumentFileName(1, "Verified Formal Complaint", ".pdf")
	want := "01_Verified Formal Complaint.pdf"
	if got != want {
		t.Errorf("DocumentFileName = %q, want %q", got, want)
	}
}

func TestExtensionForPrefersContentType(t *testing.T) {
	if got := extensionFor("application/pdf; charset=utf-8", "whatever.docx"); got != ".pdf" {
		t.Errorf("extensionFor = %q, want %q", got, ".pdf")
	}
}

func TestExtensionForFallsBackToOriginalName(t *testing.T) {
	if got := extensionFor("", "(01) Some File.docx"); got != ".docx" {
		t.Errorf("extensionFor = %q, want %q", got, ".docx")
	}
}

func TestExtensionForDefaultsToPDF(t *testing.T) {
	if got := extensionFor("", ""); got != ".pdf" {
		t.Errorf("extensionFor = %q, want %q", got, ".pdf")
	}
}
