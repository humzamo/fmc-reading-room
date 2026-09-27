package fmcsite

import (
	"strconv"
	"strings"
	"time"
)

// parseSiteDate parses the site's MM/DD/YYYY dates, returning the zero
// time for blank/unparsable input (some dates, e.g. "Last Updated", are
// legitimately unset).
func parseSiteDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("01/02/2006", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
