package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

type report struct {
	mu       sync.Mutex
	analyzer *responseAnalyzer

	total    int
	errs     int
	firstErr error
	byStatus map[int]int

	latencyCount int
	latencyTotal time.Duration
	latencyMin   time.Duration
	latencyMax   time.Duration

	markerMatchCounts    map[string]int
	markerResponseCounts map[string]int
	categoryRespCounts   map[MarkerCategory]int

	topN int
	top  []offendingResponse
}

type offendingResponse struct {
	Score           int
	StatusCode      int
	Latency         time.Duration
	MarkerIDs       []string
	PromptPreview   string
	ResponsePreview string
	Error           string
}

func newReport(analyzer *responseAnalyzer) *report {
	return &report{
		analyzer:             analyzer,
		byStatus:             make(map[int]int),
		markerMatchCounts:    make(map[string]int),
		markerResponseCounts: make(map[string]int),
		categoryRespCounts:   make(map[MarkerCategory]int),
		topN:                 10,
		latencyMin:           0,
		latencyMax:           0,
		latencyTotal:         0,
		latencyCount:         0,
	}
}

func (r *report) RecordError(err error) {
	r.RecordResult(RequestResult{Err: err})
}

func (r *report) RecordResult(res RequestResult) {
	var hits []MarkerHit
	if r.analyzer != nil && res.Err == nil {
		hits = r.analyzer.Analyze(res)
	}

	var markerIDs []string
	var totalMatches int
	categorySeen := make(map[MarkerCategory]bool, 4)
	for _, h := range hits {
		markerIDs = append(markerIDs, h.ID)
		totalMatches += h.Count
		categorySeen[h.Category] = true
	}

	score := offenseScore(len(markerIDs), totalMatches)
	var offender *offendingResponse
	if score > 0 {
		off := offendingResponse{
			Score:           score,
			StatusCode:      res.StatusCode,
			Latency:         res.Latency,
			MarkerIDs:       markerIDs,
			PromptPreview:   previewOneLine(res.Prompt, 140),
			ResponsePreview: previewOneLineBytes(res.Body, 240),
		}
		if res.Err != nil {
			off.Error = res.Err.Error()
		}
		offender = &off
	}

	var progressLog *string

	r.mu.Lock()
	r.total++

	if res.Err != nil {
		r.errs++
		if r.firstErr == nil {
			r.firstErr = res.Err
		}
	} else {
		r.byStatus[res.StatusCode]++
	}

	if res.Latency > 0 {
		r.latencyCount++
		r.latencyTotal += res.Latency
		if r.latencyMin == 0 || res.Latency < r.latencyMin {
			r.latencyMin = res.Latency
		}
		if res.Latency > r.latencyMax {
			r.latencyMax = res.Latency
		}
	}

	for _, h := range hits {
		r.markerMatchCounts[h.ID] += h.Count
		r.markerResponseCounts[h.ID]++
	}
	for c := range categorySeen {
		r.categoryRespCounts[c]++
	}

	if offender != nil {
		r.maybeAddTopLocked(*offender)
	}

	if r.total%progressEveryN == 0 {
		s := fmt.Sprintf("progress: sent=%d last_status=%d last_latency=%s", r.total, res.StatusCode, res.Latency)
		progressLog = &s
	}
	r.mu.Unlock()

	if progressLog != nil {
		log.Print(*progressLog)
	}
}

func (r *report) maybeAddTopLocked(off offendingResponse) {
	if r.topN <= 0 {
		return
	}
	r.top = append(r.top, off)
	sort.Slice(r.top, func(i, j int) bool {
		if r.top[i].Score != r.top[j].Score {
			return r.top[i].Score > r.top[j].Score
		}
		if r.top[i].Latency != r.top[j].Latency {
			return r.top[i].Latency > r.top[j].Latency
		}
		return r.top[i].StatusCode > r.top[j].StatusCode
	})
	if len(r.top) > r.topN {
		r.top = r.top[:r.topN]
	}
}

func (r *report) LogSummary() {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Printf("done: sent=%d errs=%d", r.total, r.errs)
	if r.firstErr != nil {
		log.Printf("first_error: %v", r.firstErr)
	}

	if r.latencyCount > 0 {
		avg := time.Duration(int64(r.latencyTotal) / int64(r.latencyCount))
		log.Printf("latency: min=%s avg=%s max=%s", r.latencyMin, avg, r.latencyMax)
	}

	if len(r.byStatus) > 0 {
		var codes []int
		for code := range r.byStatus {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		for _, code := range codes {
			log.Printf("status_%d: %d", code, r.byStatus[code])
		}
	}

	if len(r.categoryRespCounts) > 0 {
		var cats []string
		for c := range r.categoryRespCounts {
			cats = append(cats, string(c))
		}
		sort.Strings(cats)
		for _, c := range cats {
			log.Printf("category_%s_responses: %d", c, r.categoryRespCounts[MarkerCategory(c)])
		}
	}

	if len(r.markerResponseCounts) > 0 {
		type row struct {
			id        string
			responses int
			matches   int
		}
		rows := make([]row, 0, len(r.markerResponseCounts))
		for id, respN := range r.markerResponseCounts {
			rows = append(rows, row{id: id, responses: respN, matches: r.markerMatchCounts[id]})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].responses != rows[j].responses {
				return rows[i].responses > rows[j].responses
			}
			if rows[i].matches != rows[j].matches {
				return rows[i].matches > rows[j].matches
			}
			return rows[i].id < rows[j].id
		})
		log.Printf("markers: (responses / matches)")
		for _, r := range rows {
			log.Printf("marker_%s: %d / %d", r.id, r.responses, r.matches)
		}
	}

	if len(r.top) > 0 {
		log.Printf("top_offenders:")
		for i, off := range r.top {
			ids := strings.Join(off.MarkerIDs, ",")
			if ids == "" {
				ids = "-"
			}
			line := fmt.Sprintf("#%d score=%d status=%d latency=%s markers=%s", i+1, off.Score, off.StatusCode, off.Latency, ids)
			if off.Error != "" {
				line += " err=" + previewOneLine(off.Error, 140)
			}
			log.Print(line)
			if off.PromptPreview != "" {
				log.Printf("  prompt=%q", off.PromptPreview)
			}
			if off.ResponsePreview != "" {
				log.Printf("  resp=%q", off.ResponsePreview)
			}
		}
	}
}

func offenseScore(distinctMarkers int, totalMatches int) int {
	if distinctMarkers == 0 || totalMatches == 0 {
		return 0
	}
	// Favor responses that trip many marker types even if each only matches once.
	return distinctMarkers*2 + totalMatches
}

func previewOneLine(s string, maxChars int) string {
	if s == "" || maxChars <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxChars {
		return s
	}
	if maxChars <= 1 {
		return s[:maxChars]
	}
	return s[:maxChars-1] + "…"
}

func previewOneLineBytes(b []byte, maxChars int) string {
	if len(b) == 0 || maxChars <= 0 {
		return ""
	}
	// Avoid copying potentially large bodies; only sample a prefix.
	// Reserve some slack to survive multi-byte characters before truncation.
	maxBytes := maxChars * 4
	if maxBytes < 256 {
		maxBytes = 256
	}
	if len(b) < maxBytes {
		maxBytes = len(b)
	}
	return previewOneLine(string(b[:maxBytes]), maxChars)
}
