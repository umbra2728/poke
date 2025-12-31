package main

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

type MarkerCategory string

const (
	CategoryJailbreakSuccess MarkerCategory = "jailbreak_success"
	CategorySystemLeak       MarkerCategory = "system_leak"
	CategoryPIILeak          MarkerCategory = "pii_leak"
	CategoryCredentialLeak   MarkerCategory = "credential_leak"
	CategoryFilePathLeak     MarkerCategory = "file_path_leak"
	CategoryKeyPhraseLeak    MarkerCategory = "key_phrase_leak"
	CategoryHTTPError        MarkerCategory = "http_error"
	CategoryRateLimit        MarkerCategory = "rate_limit"
)

type MarkerHit struct {
	ID       string
	Category MarkerCategory
	Count    int
}

type markerDef struct {
	id       string
	category MarkerCategory
	re       *regexp.Regexp
	match    func(status int, headers http.Header) bool
}

type responseAnalyzer struct {
	markers []markerDef
}

func newResponseAnalyzer(cfg markerConfig) (*responseAnalyzer, error) {
	var markers []markerDef

	for _, rm := range cfg.RegexMarkers {
		if !rm.Enabled {
			continue
		}
		re, err := regexp.Compile(rm.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile regex marker %q (%s): %w", rm.ID, rm.Category, err)
		}
		markers = append(markers, markerDef{
			id:       rm.ID,
			category: rm.Category,
			re:       re,
		})
	}

	// HTTP errors (status-derived markers).
	markers = append(markers,
		statusRangeMarker(CategoryHTTPError, "http_4xx", 400, 499),
		statusRangeMarker(CategoryHTTPError, "http_5xx", 500, 599),
	)

	// Rate limiting markers.
	markers = append(markers,
		statusCodeMarker(CategoryRateLimit, "status_429", 429),
		headerPresentMarker(CategoryRateLimit, "retry_after_header", "Retry-After"),
	)

	slices.SortFunc(markers, func(a, b markerDef) int {
		if a.category != b.category {
			return strings.Compare(a.category.String(), b.category.String())
		}
		return strings.Compare(a.id, b.id)
	})

	return &responseAnalyzer{markers: markers}, nil
}

func (a *responseAnalyzer) Analyze(res RequestResult) []MarkerHit {
	if len(a.markers) == 0 {
		return nil
	}

	out := make([]MarkerHit, 0, 4)
	for _, m := range a.markers {
		var n int
		switch {
		case m.re != nil && len(res.Body) > 0:
			// Cap match counting for pathological responses.
			const maxMatches = 50
			n = len(m.re.FindAllIndex(res.Body, maxMatches))
		case m.match != nil:
			if m.match(res.StatusCode, res.Headers) {
				n = 1
			}
		}
		if n > 0 {
			out = append(out, MarkerHit{ID: m.category.String() + ":" + m.id, Category: m.category, Count: n})
		}
	}
	return out
}

func (c MarkerCategory) String() string { return string(c) }

func statusRangeMarker(category MarkerCategory, id string, min int, max int) markerDef {
	return markerDef{
		id:       id,
		category: category,
		match: func(status int, _ http.Header) bool {
			return status >= min && status <= max
		},
	}
}

func statusCodeMarker(category MarkerCategory, id string, code int) markerDef {
	return markerDef{
		id:       id,
		category: category,
		match: func(status int, _ http.Header) bool {
			return status == code
		},
	}
}

func headerPresentMarker(category MarkerCategory, id string, header string) markerDef {
	return markerDef{
		id:       id,
		category: category,
		match: func(_ int, headers http.Header) bool {
			if headers == nil {
				return false
			}
			return headers.Get(header) != ""
		},
	}
}
