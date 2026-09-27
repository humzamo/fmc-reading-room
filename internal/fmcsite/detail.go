package fmcsite

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// FetchProceedingDetail fetches https://www2.fmc.gov/readingroom/proceeding/{number}/,
// a plain (non-postback) page listing the proceeding's title, last-updated
// date, and every document filed under it. This grid never paginates
// (confirmed via its client-side settings: AllowPaging is false), so a
// single GET always returns the complete document list.
func (c *Client) FetchProceedingDetail(ctx context.Context, number string) (ProceedingDetail, error) {
	path := fmt.Sprintf("proceeding/%s/", escapeProceedingNumber(number))
	doc, err := c.getHTML(ctx, path)
	if err != nil {
		return ProceedingDetail{}, fmt.Errorf("fmcsite: fetching proceeding %s: %w", number, err)
	}
	return parseProceedingDetail(doc, number, c.BaseURL), nil
}

func parseProceedingDetail(doc *goquery.Document, number, baseURL string) ProceedingDetail {
	detail := ProceedingDetail{Number: number}

	titleText := strings.TrimSpace(doc.Find("#MainContent_lblTitle").Text())
	detail.Title = strings.TrimSpace(strings.TrimPrefix(titleText, number+" - "))

	lastUpdatedText := strings.TrimSpace(doc.Find("#MainContent_lblLastUpdated").Text())
	lastUpdatedText = strings.TrimSpace(strings.TrimPrefix(lastUpdatedText, "Last Updated:"))
	detail.LastUpdated = parseSiteDate(lastUpdatedText)

	doc.Find("table.rgMasterTable > tbody > tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.ChildrenFiltered("td")
		if cells.Length() < 9 {
			return // header/pager rows, or an unexpected layout
		}
		docID := strings.TrimSpace(cells.Eq(8).Text())
		if docID == "" {
			return
		}
		detail.Documents = append(detail.Documents, DocumentRow{
			Number:      parseInt(cells.Eq(0).Text()),
			ServedDate:  parseSiteDate(cells.Eq(1).Text()),
			Description: strings.TrimSpace(cells.Eq(2).Text()),
			SourceURL:   baseURL + "/documents/" + docID,
		})
	})

	return detail
}

// escapeProceedingNumber makes a proceeding number safe to embed as a URL
// path segment. Numbers like "2049(I)" or "SP-011981" contain characters
// that are safe unescaped in practice on this site, but "/" (theoretically
// possible in a title-like field) must never be allowed to reshape the
// path, so this is a conservative allowlist rather than full URL-encoding
// (which the site's own links don't do, and which 404s in testing).
func escapeProceedingNumber(number string) string {
	return strings.ReplaceAll(number, "/", "%2F")
}
