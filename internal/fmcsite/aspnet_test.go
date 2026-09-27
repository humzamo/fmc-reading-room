package fmcsite

import (
	"os"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func loadFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("opening fixture %s: %v", name, err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("parsing fixture %s: %v", name, err)
	}
	return doc
}

func TestParsePostbackForm(t *testing.T) {
	doc := loadFixture(t, "proceeding_search_results.html")
	form := parsePostbackForm(doc)

	for _, field := range []string{"__VIEWSTATE", "__VIEWSTATEGENERATOR", "__EVENTVALIDATION"} {
		if form.values.Get(field) == "" {
			t.Errorf("expected field %s to be captured, got empty", field)
		}
	}
	// Submit/image buttons must not be captured automatically.
	if _, ok := form.values["ctl00$MainContent$RadGrid1$ctl00$ctl03$ctl01$ctl12"]; ok {
		t.Errorf("expected Next Page submit button to be excluded until clicked")
	}
}

func TestNextPageButtonName(t *testing.T) {
	doc := loadFixture(t, "proceeding_search_results.html")
	name, enabled := nextPageButtonName(doc)
	wantName := "ctl00$MainContent$RadGrid1$ctl00$ctl03$ctl01$ctl12"
	if name != wantName {
		t.Errorf("name = %q, want %q", name, wantName)
	}
	if !enabled {
		t.Errorf("expected Next Page button to be enabled on page 1 of a 3-page result")
	}
}

func TestWithProceedingType(t *testing.T) {
	f := postbackForm{values: map[string][]string{}}.withProceedingType(ProceedingTypeDockets)
	got := f.values.Get(fieldProceedingTypeClientState)
	want := `{"enabled":true,"logEntries":[],"selectedIndex":1,"selectedText":"Dockets","selectedValue":"1"}`
	if got != want {
		t.Errorf("ClientState = %s, want %s", got, want)
	}
}

func TestWithIsClosedYes(t *testing.T) {
	f := postbackForm{values: map[string][]string{}}.withIsClosedYes()
	got := f.values.Get(fieldIsClosedClientState)
	want := `{"enabled":true,"logEntries":[],"selectedIndex":2,"selectedText":"Yes","selectedValue":"1"}`
	if got != want {
		t.Errorf("ClientState = %s, want %s", got, want)
	}
}

func TestClickSetsButtonAndClearsEventTarget(t *testing.T) {
	f := postbackForm{values: map[string][]string{}}
	f.values.Set("__EVENTTARGET", "something")
	f = f.clickSearch()
	if f.values.Get(fieldSearchButton) == "" {
		t.Errorf("expected search button field to be set")
	}
	if f.values.Get("__EVENTTARGET") != "" {
		t.Errorf("expected __EVENTTARGET to be cleared when clicking a real submit button")
	}
}
