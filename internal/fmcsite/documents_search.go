package fmcsite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const documentSearchPath = "DocumentSearch"

// SiteDocument is one row from the site-wide DocumentSearch grid: unlike
// FetchProceedingDetail (one proceeding's full document list),
// SearchDocumentsSince spans every proceeding at once, filtered by date.
type SiteDocument struct {
	ProceedingNumber string
	Number           int
	ServedDate       time.Time
	Description      string
	SourceURL        string
}

// SearchDocumentsSince returns every document served on or after since,
// across all proceedings, using the site's own "Document Serve Date From"
// filter. This is far cheaper than fetching every proceeding's detail page
// when only a small delta is expected — and because it filters by the
// document's own serve date rather than by which proceeding it belongs to,
// it correctly finds a brand-new filing on an old, otherwise-dormant
// proceeding just as reliably as one on a proceeding created yesterday.
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

// parseDocumentSearchRows reads the grid's columns by position, matching
// the header order confirmed on the live site: Proceeding Number, Document
// Number, Document Serve Date, Title (a display-only concatenation of the
// proceeding's title and the document's own description — not used here),
// DocTitle (hidden; the document's clean description), FileType, FileName,
// ProceedingNumber (hidden duplicate), Document link, Document button,
// DocumentId (hidden), DocketId (hidden).
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
