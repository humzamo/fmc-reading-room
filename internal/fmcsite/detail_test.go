package fmcsite

import "testing"

func TestParseProceedingDetail(t *testing.T) {
	doc := loadFixture(t, "proceeding_26-12.html")
	detail := parseProceedingDetail(doc, "26-12", "https://www2.fmc.gov/readingroom")

	if detail.Title != "Samsung Electronics America, Inc. v. CMA CGM S.A." {
		t.Errorf("Title = %q", detail.Title)
	}
	if detail.LastUpdated.Format("01/02/2006") != "09/09/2026" {
		t.Errorf("LastUpdated = %v", detail.LastUpdated)
	}
	if len(detail.Documents) != 3 {
		t.Fatalf("len(Documents) = %d, want 3", len(detail.Documents))
	}

	first := detail.Documents[0]
	if first.Number != 1 {
		t.Errorf("Documents[0].Number = %d, want 1", first.Number)
	}
	if first.Description != "Verified Formal Complaint" {
		t.Errorf("Documents[0].Description = %q", first.Description)
	}
	if first.SourceURL != "https://www2.fmc.gov/readingroom/documents/141820" {
		t.Errorf("Documents[0].SourceURL = %q", first.SourceURL)
	}
	if first.ServedDate.Format("01/02/2006") != "08/24/2026" {
		t.Errorf("Documents[0].ServedDate = %v", first.ServedDate)
	}

	last := detail.Documents[2]
	if last.Number != 9 {
		t.Errorf("Documents[2].Number = %d, want 9", last.Number)
	}
	// Description text should be flattened even when wrapped in <strong>.
	if last.Description != "Served Initial Order" {
		t.Errorf("Documents[2].Description = %q", last.Description)
	}
}
