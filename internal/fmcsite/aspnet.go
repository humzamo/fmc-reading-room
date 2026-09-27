package fmcsite

import (
	"fmt"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// postbackForm snapshots every named <input> on an ASP.NET WebForms page so
// it can be resubmitted, optionally after mutating a few fields, to
// simulate a postback. Submit/image inputs are excluded by default and
// only added via click(), matching how a browser only sends the one button
// that was pressed.
type postbackForm struct {
	values url.Values
}

// parsePostbackForm captures a response's form state. Call it on every
// response before deriving the next request: __VIEWSTATE/__EVENTVALIDATION
// change on every postback.
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

func (f postbackForm) set(name, value string) postbackForm {
	f.values.Set(name, value)
	return f
}

// click simulates pressing a submit/image button by name.
func (f postbackForm) click(name string) postbackForm {
	f.values.Set(name, " ")
	return f
}

// clearEventTarget resets __EVENTTARGET/__EVENTARGUMENT; needed before
// click(), since real submit buttons don't use them.
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

	// DocumentSearch's "Document Serve Date From" (a Telerik RadDatePicker):
	// both the display text and its ClientState JSON must be set.
	fieldServeDateFromDisplay     = "ctl00$MainContent$rdpDocumentServeFromDate$dateInput"
	fieldServeDateFromClientState = "ctl00_MainContent_rdpDocumentServeFromDate_dateInput_ClientState"
)

// withProceedingType sets the ddlProceedingType filter via its
// RadDropDownList ClientState JSON — this control has no plain <select>.
func (f postbackForm) withProceedingType(t ProceedingType) postbackForm {
	idx, val := t.ddlIndexValue()
	cs := fmt.Sprintf(`{"enabled":true,"logEntries":[],"selectedIndex":%d,"selectedText":%q,"selectedValue":%q}`, idx, string(t), val)
	return f.set(fieldProceedingTypeClientState, cs)
}

// withIsClosedYes sets the ddlClosed filter to "Yes" (index 2, value "1").
func (f postbackForm) withIsClosedYes() postbackForm {
	cs := `{"enabled":true,"logEntries":[],"selectedIndex":2,"selectedText":"Yes","selectedValue":"1"}`
	return f.set(fieldIsClosedClientState, cs)
}

// withServeDateFrom sets DocumentSearch's "Document Serve Date From" filter.
func (f postbackForm) withServeDateFrom(since time.Time) postbackForm {
	display := since.Format("1/2/2006")
	iso := since.Format("2006-01-02") + "-00-00-00"
	clientState := fmt.Sprintf(
		`{"enabled":true,"emptyMessage":"","validationText":%q,"valueAsString":%q,"minDateStr":"1970-01-01-00-00-00","maxDateStr":"2050-01-01-00-00-00","lastSetTextBoxValue":%q}`,
		iso, iso, display)
	return f.set(fieldServeDateFromDisplay, display).set(fieldServeDateFromClientState, clientState)
}

func (f postbackForm) clickSearch() postbackForm {
	return f.clearEventTarget().click(fieldSearchButton)
}

// nextPageButtonName returns the pager's "Next Page" button name and
// whether it's enabled. A disabled button (last page) still renders but
// with onclick="return false;".
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

func (f postbackForm) clickNextPage(buttonName string) postbackForm {
	return f.clearEventTarget().click(buttonName)
}
