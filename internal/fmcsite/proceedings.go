package fmcsite

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const proceedingSearchPath = "ProceedingSearch"

// maxSearchPages guards against an infinite loop if "next page" detection
// is ever wrong; far beyond anything the ~700-item proceeding list needs.
const maxSearchPages = 500

// DiscoveredProceeding is a ProceedingSummary enriched with type/closed
// (see DiscoverProceedings).
type DiscoveredProceeding struct {
	ProceedingSummary
	Type     ProceedingType
	IsClosed bool
}

// DiscoverProceedings finds every proceeding on the site, with its type and
// closed status where known — neither is shown anywhere except as a search
// filter, so they're derived by running one search per Proceeding Type plus
// one for Is Closed = Yes, tagging matches. An unfiltered search also runs:
// confirmed live, at least one proceeding ("23-10") has no type set and
// returns "No records to display" under every specific type filter, so the
// type searches alone would miss it.
func (c *Client) DiscoverProceedings(ctx context.Context) ([]DiscoveredProceeding, error) {
	byNumber := make(map[string]*DiscoveredProceeding)
	var order []string

	addRow := func(row ProceedingSummary, t ProceedingType) {
		if existing, ok := byNumber[row.Number]; ok {
			if t != "" {
				existing.Type = t
			}
			return
		}
		order = append(order, row.Number)
		byNumber[row.Number] = &DiscoveredProceeding{ProceedingSummary: row, Type: t}
	}

	for _, t := range AllProceedingTypes {
		rows, err := c.searchProceedings(ctx, func(f postbackForm) postbackForm {
			return f.withProceedingType(t)
		})
		if err != nil {
			return nil, fmt.Errorf("fmcsite: discovering proceedings of type %q: %w", t, err)
		}
		for _, row := range rows {
			addRow(row, t)
		}
	}

	allRows, err := c.searchProceedings(ctx, func(f postbackForm) postbackForm { return f })
	if err != nil {
		return nil, fmt.Errorf("fmcsite: discovering proceedings (unfiltered catch-all): %w", err)
	}
	for _, row := range allRows {
		addRow(row, "")
	}

	closedRows, err := c.searchProceedings(ctx, func(f postbackForm) postbackForm {
		return f.withIsClosedYes()
	})
	if err != nil {
		return nil, fmt.Errorf("fmcsite: discovering closed proceedings: %w", err)
	}
	for _, row := range closedRows {
		if p, ok := byNumber[row.Number]; ok {
			p.IsClosed = true
		} else {
			// Shouldn't happen, but don't drop it if it does.
			order = append(order, row.Number)
			byNumber[row.Number] = &DiscoveredProceeding{ProceedingSummary: row, IsClosed: true}
		}
	}

	result := make([]DiscoveredProceeding, 0, len(order))
	for _, number := range order {
		result = append(result, *byNumber[number])
	}
	return result, nil
}

// searchProceedings runs the ProceedingSearch grid with a filter applied via
// mutateForm, following pagination until the last page, and returns every
// row found.
func (c *Client) searchProceedings(ctx context.Context, mutateForm func(postbackForm) postbackForm) ([]ProceedingSummary, error) {
	doc, err := c.getHTML(ctx, proceedingSearchPath)
	if err != nil {
		return nil, fmt.Errorf("loading search page: %w", err)
	}

	form := mutateForm(parsePostbackForm(doc)).clickSearch()
	doc, err = c.postForm(ctx, proceedingSearchPath, form)
	if err != nil {
		return nil, fmt.Errorf("submitting search: %w", err)
	}

	var results []ProceedingSummary
	for page := 1; page <= maxSearchPages; page++ {
		results = append(results, parseProceedingSearchRows(doc)...)

		name, enabled := nextPageButtonName(doc)
		if !enabled {
			return results, nil
		}
		if page%10 == 0 {
			c.progress("proceeding search: page %d done, %d rows so far", page, len(results))
		}
		form = parsePostbackForm(doc).clickNextPage(name)
		doc, err = c.postForm(ctx, proceedingSearchPath, form)
		if err != nil {
			return nil, fmt.Errorf("fetching search page %d: %w", page+1, err)
		}
	}
	return nil, fmt.Errorf("exceeded %d pages without reaching the last page", maxSearchPages)
}

func parseProceedingSearchRows(doc *goquery.Document) []ProceedingSummary {
	var rows []ProceedingSummary
	doc.Find("table.rgMasterTable > tbody > tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.ChildrenFiltered("td")
		if cells.Length() < 5 {
			return
		}
		number := strings.TrimSpace(cells.Eq(0).Text())
		if number == "" {
			return
		}
		rows = append(rows, ProceedingSummary{
			Number:      number,
			Title:       strings.TrimSpace(cells.Eq(1).Text()),
			CreatedDate: parseSiteDate(cells.Eq(4).Text()),
		})
	})
	return rows
}
