package fmcsite

import (
	"fmt"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// postbackForm is a snapshot of every named <input> value on an ASP.NET
// WebForms page, ready to be resubmitted (optionally after mutating a few
// fields) to simulate the next postback — a page change, a filter change,
// or clicking a button. Submit/image inputs are excluded by default: they're
// only included in a request when explicitly "clicked" via click(), exactly
// like a real browser only sends the one button that was pressed.
type postbackForm struct {
	values url.Values
}

// parsePostbackForm captures the current state of the page's form so it can
// be resubmitted. Call this on every response before deriving the next
// request, since __VIEWSTATE/__EVENTVALIDATION change on every postback.
func parsePostbackForm(doc *goquery.Document) postbackForm {
	v := url.Values{}
	doc.Find("input[name]").Each(func(_ int, s *goquery.Selection) {
		typ, _ := s.Attr("type")
		if typ == "submit" || typ == "image" || typ == "button" {
			return
		}
		name, _ := s.Attr("name")
		val, _ := s.Attr("value")
		v.Set(name, val)
	})
	return postbackForm{values: v}
}

func (f postbackForm) clone() postbackForm {
	cloned := url.Values{}
	for k, v := range f.values {
		cloned[k] = append([]string(nil), v...)
	}
	return postbackForm{values: cloned}
}

// set overwrites a field's value (used for search filter fields).
func (f postbackForm) set(name, value string) postbackForm {
	f.values.Set(name, value)
	return f
}

// click adds a submit/image button's name=value pair, simulating pressing
// that specific button (e.g. "Search" or the pager's "Next Page" button).
func (f postbackForm) click(name string) postbackForm {
	f.values.Set(name, " ")
	return f
}

// clearEventTarget resets the generic postback target fields; needed before
// "clicking" a real submit button, since those don't use __EVENTTARGET.
func (f postbackForm) clearEventTarget() postbackForm {
	f.values.Set("__EVENTTARGET", "")
	f.values.Set("__EVENTARGUMENT", "")
	return f
}

const (
	fieldProceedingTypeClientState = "ctl00_MainContent_ddlProceedingType_ClientState"
	fieldIsClosedClientState       = "ctl00_MainContent_ddlClosed_ClientState"
	fieldSearchButton              = "ctl00$MainContent$btnSearch"
	fieldNextPageButtonTitle       = "Next Page"

	// DocumentSearch's "Document Serve Date From" filter, a Telerik
	// RadDatePicker. Its display text input and its ClientState hidden
	// field (which is what the server actually parses) must both be set.
	fieldServeDateFromDisplay     = "ctl00$MainContent$rdpDocumentServeFromDate$dateInput"
	fieldServeDateFromClientState = "ctl00_MainContent_rdpDocumentServeFromDate_dateInput_ClientState"
)

// withProceedingType sets the ddlProceedingType filter to the given value by
// writing the RadDropDownList's ClientState JSON the server expects on
// postback (this control has no plain <select>; its selection only exists
// in this hidden field).
func (f postbackForm) withProceedingType(t ProceedingType) postbackForm {
	idx, val := t.ddlIndexValue()
	cs := fmt.Sprintf(`{"enabled":true,"logEntries":[],"selectedIndex":%d,"selectedText":%q,"selectedValue":%q}`, idx, string(t), val)
	return f.set(fieldProceedingTypeClientState, cs)
}

// withIsClosed sets the ddlClosed filter. index 2 / value "1" is "Yes",
// per the widget's item data on the live search page.
func (f postbackForm) withIsClosedYes() postbackForm {
	cs := `{"enabled":true,"logEntries":[],"selectedIndex":2,"selectedText":"Yes","selectedValue":"1"}`
	return f.set(fieldIsClosedClientState, cs)
}

// withServeDateFrom sets DocumentSearch's "Document Serve Date From" filter.
// Confirmed against the live site: setting these two fields and submitting
// reproduces exactly what typing a date into the picker and clicking
// Search does (verified by comparing the resulting item count).
func (f postbackForm) withServeDateFrom(since time.Time) postbackForm {
	display := since.Format("1/2/2006")
	iso := since.Format("2006-01-02") + "-00-00-00"
	clientState := fmt.Sprintf(
		`{"enabled":true,"emptyMessage":"","validationText":%q,"valueAsString":%q,"minDateStr":"1970-01-01-00-00-00","maxDateStr":"2050-01-01-00-00-00","lastSetTextBoxValue":%q}`,
		iso, iso, display)
	return f.set(fieldServeDateFromDisplay, display).set(fieldServeDateFromClientState, clientState)
}

// clickSearch simulates pressing the Search button.
func (f postbackForm) clickSearch() postbackForm {
	return f.clearEventTarget().click(fieldSearchButton)
}

// nextPageButtonName returns the pager's "Next Page" submit button's name,
// and whether it's currently enabled. A disabled Next Page button (present
// but non-functional, rendered with onclick="return false;") means we're on
// the grid's last page.
func nextPageButtonName(doc *goquery.Document) (name string, enabled bool) {
	sel := doc.Find(`input[type="submit"][title="` + fieldNextPageButtonTitle + `"]`)
	if sel.Length() == 0 {
		return "", false
	}
	name, _ = sel.Attr("name")
	if onclick, _ := sel.Attr("onclick"); onclick != "" {
		return name, false
	}
	return name, true
}

// clickNextPage advances the grid to the next page, given the button name
// found via nextPageButtonName.
func (f postbackForm) clickNextPage(buttonName string) postbackForm {
	return f.clearEventTarget().click(buttonName)
}
