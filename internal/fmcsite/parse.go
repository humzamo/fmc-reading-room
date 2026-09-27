package fmcsite

import (
	"strconv"
	"strings"
	"time"
)

// parseSiteDate parses the MM/DD/YYYY format used throughout the reading
// room's pages. Returns the zero time if s is empty or unparsable, since
// some dates are legitimately blank (e.g. an unset "Last Updated").
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
