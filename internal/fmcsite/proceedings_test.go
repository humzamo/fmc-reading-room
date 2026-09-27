package fmcsite

import "testing"

func TestParseProceedingSearchRows(t *testing.T) {
	doc := loadFixture(t, "proceeding_search_results.html")
	rows := parseProceedingSearchRows(doc)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	first := rows[0]
	if first.Number != "2066(I)" {
		t.Errorf("rows[0].Number = %q, want %q", first.Number, "2066(I)")
	}
	if first.Title != "Naxa Electronics, Inc. v. Ocean Network Express Pte. Ltd.; Orient Overseas Container Line Limited; and Sinpex Connection Logistics Limited" {
		t.Errorf("rows[0].Title = %q", first.Title)
	}
	if first.CreatedDate.Format("01/02/2006") != "06/16/2026" {
		t.Errorf("rows[0].CreatedDate = %v", first.CreatedDate)
	}

	second := rows[1]
	if second.Number != "2055(I)" {
		t.Errorf("rows[1].Number = %q, want %q", second.Number, "2055(I)")
	}
}
