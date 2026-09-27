package fmcsite

import "testing"

func TestParseDocumentSearchRows(t *testing.T) {
	doc := loadFixture(t, "document_search_results.html")
	rows := parseDocumentSearchRows(doc, "https://www2.fmc.gov/readingroom")

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	row := rows[0]
	if row.ProceedingNumber != "2049(I)" {
		t.Errorf("ProceedingNumber = %q, want %q", row.ProceedingNumber, "2049(I)")
	}
	if row.Number != 2 {
		t.Errorf("Number = %d, want 2", row.Number)
	}
	if row.ServedDate.Format("01/02/2006") != "09/25/2026" {
		t.Errorf("ServedDate = %v", row.ServedDate)
	}
	// Must come from the hidden DocTitle column, not the concatenated
	// visible Title column.
	if row.Description != "Served Notice of Commission Determination to Review" {
		t.Errorf("Description = %q", row.Description)
	}
	if row.SourceURL != "https://www2.fmc.gov/readingroom/documents/144909" {
		t.Errorf("SourceURL = %q", row.SourceURL)
	}
}

func TestWithServeDateFrom(t *testing.T) {
	f := postbackForm{values: map[string][]string{}}
	since := mustParseDate(t, "2026-01-01")
	f = f.withServeDateFrom(since)

	if got := f.values.Get(fieldServeDateFromDisplay); got != "1/1/2026" {
		t.Errorf("display field = %q, want %q", got, "1/1/2026")
	}
	want := `{"enabled":true,"emptyMessage":"","validationText":"2026-01-01-00-00-00","valueAsString":"2026-01-01-00-00-00","minDateStr":"1970-01-01-00-00-00","maxDateStr":"2050-01-01-00-00-00","lastSetTextBoxValue":"1/1/2026"}`
	if got := f.values.Get(fieldServeDateFromClientState); got != want {
		t.Errorf("ClientState = %s, want %s", got, want)
	}
}
