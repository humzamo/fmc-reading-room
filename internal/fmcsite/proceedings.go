package fmcsite

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const proceedingSearchPath = "ProceedingSearch"

// maxSearchPages guards against an infinite loop if the pager's "next page"
// detection is ever wrong (e.g. a site markup change); 500 pages is far
// beyond anything the ~700-item proceeding list should ever need.
const maxSearchPages = 500

// DiscoveredProceeding is a ProceedingSummary enriched with the type/closed
// classification derived from filtered searches (see DiscoverProceedings).
type DiscoveredProceeding struct {
	ProceedingSummary
	Type     ProceedingType
	IsClosed bool
}

// DiscoverProceedings finds every proceeding currently on the site, along
// with its type and closed status. Those two fields aren't shown anywhere
// except as search filter values, so they're derived by running one
// paginated search per Proceeding Type (whose union is every proceeding)
// plus one more paginated search for Is Closed = Yes, tagging matches.
func (c *Client) DiscoverProceedings(ctx context.Context) ([]DiscoveredProceeding, error) {
	byNumber := make(map[string]*DiscoveredProceeding)
	var order []string

	for _, t := range AllProceedingTypes {
		rows, err := c.searchProceedings(ctx, func(f postbackForm) postbackForm {
			return f.withProceedingType(t)
		})
		if err != nil {
			return nil, fmt.Errorf("fmcsite: discovering proceedings of type %q: %w", t, err)
		}
		for _, row := range rows {
			if _, exists := byNumber[row.Number]; !exists {
				order = append(order, row.Number)
			}
			byNumber[row.Number] = &DiscoveredProceeding{ProceedingSummary: row, Type: t}
		}
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
			// Shouldn't happen (every proceeding should have a type), but
			// don't silently drop it if the site's data is inconsistent.
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
