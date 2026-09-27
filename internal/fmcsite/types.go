// Package fmcsite knows how to talk to the FMC reading room website
// (https://www2.fmc.gov/readingroom/): an ASP.NET WebForms app whose search
// grids page via classic postbacks, but whose per-proceeding detail pages
// are plain, unpaginated GETs.
package fmcsite

import "time"

// ProceedingType is one of the enum values the ProceedingSearch page's
// "Proceeding Type" filter accepts. The site doesn't expose this value
// anywhere except as a search filter, so it's derived by tagging every
// proceeding number returned by each type-filtered search.
type ProceedingType string

const (
	ProceedingTypeDockets              ProceedingType = "Dockets"
	ProceedingTypePetition             ProceedingType = "Petition"
	ProceedingTypeNoticeOfInquiry      ProceedingType = "Notice of Inquiry"
	ProceedingTypeFactFinding          ProceedingType = "Fact Finding"
	ProceedingTypeSpecialPermission    ProceedingType = "Special Permission"
	ProceedingTypeSpecialInvestigation ProceedingType = "Special Investigation"
)

// AllProceedingTypes lists every filter value, in the order the site's
// dropdown presents them (index matters for the RadDropDownList ClientState
// payload built in aspnet.go).
var AllProceedingTypes = []ProceedingType{
	ProceedingTypeDockets,
	ProceedingTypePetition,
	ProceedingTypeNoticeOfInquiry,
	ProceedingTypeFactFinding,
	ProceedingTypeSpecialPermission,
	ProceedingTypeSpecialInvestigation,
}

// ddlIndexValue returns the RadDropDownList item index and underlying value
// the site's ddlProceedingType control uses for this type, as read from the
// widget's client-side item data on the live search page.
func (t ProceedingType) ddlIndexValue() (index int, value string) {
	switch t {
	case ProceedingTypeDockets:
		return 1, "1"
	case ProceedingTypePetition:
		return 2, "2"
	case ProceedingTypeNoticeOfInquiry:
		return 3, "8"
	case ProceedingTypeFactFinding:
		return 4, "9"
	case ProceedingTypeSpecialPermission:
		return 5, "10"
	case ProceedingTypeSpecialInvestigation:
		return 6, "12"
	default:
		return 0, "0"
	}
}

// ProceedingSummary is one row from the ProceedingSearch grid: enough to
// discover that a proceeding exists and upsert its bookkeeping fields.
type ProceedingSummary struct {
	Number      string
	Title       string
	CreatedDate time.Time // zero if unparsable
}

// ProceedingDetail is the full content of a proceeding's detail page
// (GET /readingroom/proceeding/{number}/).
type ProceedingDetail struct {
	Number      string
	Title       string
	LastUpdated time.Time // zero if unparsable
	Documents   []DocumentRow
}

// DocumentRow is one row from a proceeding detail page's document table.
type DocumentRow struct {
	Number      int
	ServedDate  time.Time // zero if unparsable
	Description string
	// SourceURL is the absolute URL to download the document from.
	SourceURL string
}
