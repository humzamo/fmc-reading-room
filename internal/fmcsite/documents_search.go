package fmcsite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const documentSearchPath = "DocumentSearch"

// SiteDocument is one row from the site-wide DocumentSearch grid.
type SiteDocument struct {
	ProceedingNumber string
	Number           int
	ServedDate       time.Time
	Description      string
	SourceURL        string
}

// SearchDocumentsSince returns every document served on or after since,
// across all proceedings, using the site's own date filter — filtered by
// serve date, not by proceeding, so it finds new filings on old proceedings
// too.
func (c *Client) SearchDocumentsSince(ctx context.Context, since time.Time) ([]SiteDocument, error) {
	doc, err := c.getHTML(ctx, documentSearchPath)
	if err != nil {
		return nil, fmt.Errorf("fmcsite: loading document search page: %w", err)
	}

	form := parsePostbackForm(doc).withServeDateFrom(since).clickSearch()
	doc, err = c.postForm(ctx, documentSearchPath, form)
	if err != nil {
		return nil, fmt.Errorf("fmcsite: submitting document search: %w", err)
	}

	var results []SiteDocument
	for page := 1; page <= maxSearchPages; page++ {
		results = append(results, parseDocumentSearchRows(doc, c.BaseURL)...)

		name, enabled := nextPageButtonName(doc)
		if !enabled {
			return results, nil
		}
		c.progress("document search: page %d done, %d documents so far", page, len(results))
		form = parsePostbackForm(doc).clickNextPage(name)
		doc, err = c.postForm(ctx, documentSearchPath, form)
		if err != nil {
			return nil, fmt.Errorf("fmcsite: fetching document search page %d: %w", page+1, err)
		}
	}
	return nil, fmt.Errorf("fmcsite: exceeded %d pages without reaching the last page", maxSearchPages)
}

// parseDocumentSearchRows reads columns by position: [0] Proceeding
// Number, [1] Document Number, [2] Serve Date, [3] Title (display-only
// concatenation, unused), [4] DocTitle (clean description), ... [10]
// DocumentId.
func parseDocumentSearchRows(doc *goquery.Document, baseURL string) []SiteDocument {
	var rows []SiteDocument
	doc.Find("table.rgMasterTable > tbody > tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.ChildrenFiltered("td")
		if cells.Length() < 11 {
			return // header/pager rows, or an unexpected layout
		}
		proceedingNumber := strings.TrimSpace(cells.Eq(0).Text())
		docID := strings.TrimSpace(cells.Eq(10).Text())
		if proceedingNumber == "" || docID == "" {
			return
		}
		rows = append(rows, SiteDocument{
			ProceedingNumber: proceedingNumber,
			Number:           parseInt(cells.Eq(1).Text()),
			ServedDate:       parseSiteDate(cells.Eq(2).Text()),
			Description:      strings.TrimSpace(cells.Eq(4).Text()),
			SourceURL:        baseURL + "/documents/" + docID,
		})
	})
	return rows
}
