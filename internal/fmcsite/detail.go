package fmcsite

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// FetchProceedingDetail fetches https://www2.fmc.gov/readingroom/proceeding/{number}/:
// a plain (non-postback) page listing the proceeding's title, last-updated
// date, and every document filed under it. This grid never paginates, so a
// single GET always returns the complete list.
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

// escapeProceedingNumber only escapes "/", so a number can't reshape the
// URL path. Full URL-encoding 404s on this site; other characters (e.g.
// "2049(I)") are safe unescaped.
func escapeProceedingNumber(number string) string {
	return strings.ReplaceAll(number, "/", "%2F")
}
