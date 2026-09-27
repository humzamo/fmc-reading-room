package fmcsite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ErrNotFound indicates the server returned 404 for a document. This is
// treated specially by the sync package: it means the reading room's own
// listing links to a document that no longer resolves, which retrying
// won't fix — the sync package records it as permanently unavailable
// rather than trying it again on every future run.
var ErrNotFound = errors.New("fmcsite: document not found (404)")

const defaultUserAgent = "fmc-reading-room-sync/1.0 (+local research tool; low-volume, polite crawl)"

// Client talks to the FMC reading room. It is not safe for concurrent use
// across goroutines that share postback state (e.g. two concurrent grid
// crawls), but concurrent plain GETs (detail pages, downloads) are fine.
type Client struct {
	BaseURL    string // e.g. https://www2.fmc.gov/readingroom
	HTTPClient *http.Client
	UserAgent  string

	// RequestDelay is slept before every outgoing request, as a simple
	// politeness rate limit.
	RequestDelay time.Duration

	// MaxRetries is the number of extra attempts made after a failed
	// request (network error or 5xx) before giving up.
	MaxRetries int

	// Progress, if set, receives human-readable progress messages during
	// long paginated crawls (ProceedingSearch, DocumentSearch). Without
	// this, nothing is logged until an entire multi-page search finishes,
	// which can be several minutes for a wide date range and looks
	// indistinguishable from a hang.
	Progress func(format string, args ...any)
}

func (c *Client) progress(format string, args ...any) {
	if c.Progress != nil {
		c.Progress(format, args...)
	}
}

// NewClient builds a Client with sensible defaults: a cookie jar (the site
// is a classic ASP.NET session app), a generic descriptive User-Agent, a
// small delay between requests, and a few retries on transient failures.
func NewClient(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("fmcsite: creating cookie jar: %w", err)
	}
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
		UserAgent:    defaultUserAgent,
		RequestDelay: 250 * time.Millisecond,
		MaxRetries:   3,
	}, nil
}

func (c *Client) resolve(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return c.BaseURL + "/" + strings.TrimPrefix(path, "/")
}

// doWithRetry performs req.build(), retrying transient failures with a
// small linear backoff. build is called again on every attempt so callers
// can hand in a fresh *http.Request each time (request bodies can't be
// reused after being read).
func (c *Client) doWithRetry(ctx context.Context, build func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		if c.RequestDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.RequestDelay):
			}
		}

		req, err := build()
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %s", resp.Status)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("fmcsite: request failed after %d attempts: %w", c.MaxRetries+1, lastErr)
}

// getHTML fetches path and parses it as HTML.
func (c *Client) getHTML(ctx context.Context, path string) (*goquery.Document, error) {
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fmcsite: GET %s: unexpected status %s", path, resp.Status)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

// postForm submits path with the given form-encoded body (a postbackForm's
// values) and parses the HTML response.
func (c *Client) postForm(ctx context.Context, path string, form postbackForm) (*goquery.Document, error) {
	body := form.values.Encode()
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve(path), strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fmcsite: POST %s: unexpected status %s", path, resp.Status)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

// DownloadResult carries the outcome of fetching a document's file.
type DownloadResult struct {
	ContentType string
	FileName    string // suggested filename from Content-Disposition, if any
	Size        int64
}

// Download streams sourceURL's body into w, returning basic response
// metadata useful for bookkeeping (content type, size, original filename).
func (c *Client) Download(ctx context.Context, sourceURL string, w io.Writer) (DownloadResult, error) {
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(sourceURL), nil)
	})
	if err != nil {
		return DownloadResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return DownloadResult{}, fmt.Errorf("fmcsite: download %s: %w", sourceURL, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return DownloadResult{}, fmt.Errorf("fmcsite: download %s: unexpected status %s", sourceURL, resp.Status)
	}

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("fmcsite: download %s: %w", sourceURL, err)
	}

	result := DownloadResult{
		ContentType: resp.Header.Get("Content-Type"),
		Size:        n,
	}
	if _, filename, err := parseContentDisposition(resp.Header.Get("Content-Disposition")); err == nil {
		result.FileName = filename
	}
	return result, nil
}

func parseContentDisposition(header string) (disposition string, filename string, err error) {
	if header == "" {
		return "", "", fmt.Errorf("empty header")
	}
	// Content-Disposition: inline; filename=...
	parts := strings.SplitN(header, ";", 2)
	disposition = strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return disposition, "", nil
	}
	for _, kv := range strings.Split(parts[1], ";") {
		kv = strings.TrimSpace(kv)
		if name, val, ok := strings.Cut(kv, "="); ok && strings.EqualFold(strings.TrimSpace(name), "filename") {
			val = strings.Trim(strings.TrimSpace(val), `"`)
			if decoded, err := url.QueryUnescape(val); err == nil {
				val = decoded
			}
			return disposition, val, nil
		}
	}
	return disposition, "", nil
}
