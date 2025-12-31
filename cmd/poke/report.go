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
	cancel   func(error)

	total    int
	errs     int
	firstErr error
	byStatus map[int]int
	retried  int
	retries  int

	latencyCount int
	latencyTotal time.Duration
	latencyMin   time.Duration
	latencyMax   time.Duration

	markerMatchCounts    map[string]int
	markerResponseCounts map[string]int
	categoryRespCounts   map[MarkerCategory]int
	categoryMatchCounts  map[MarkerCategory]int

	categoryPolicy map[MarkerCategory]categoryPolicy
	maxSeverity    severityLevel
	stopErr        error
	elevated       map[MarkerCategory]bool

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

func newReport(analyzer *responseAnalyzer, policy map[MarkerCategory]categoryPolicy, cancel func(error)) *report {
	if policy == nil {
		policy = defaultMarkerConfig().Categories
	}
	return &report{
		analyzer:             analyzer,
		cancel:               cancel,
		byStatus:             make(map[int]int),
		markerMatchCounts:    make(map[string]int),
		markerResponseCounts: make(map[string]int),
		categoryRespCounts:   make(map[MarkerCategory]int),
		categoryMatchCounts:  make(map[MarkerCategory]int),
		categoryPolicy:       policy,
		maxSeverity:          severityInfo,
		elevated:             make(map[MarkerCategory]bool),
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
	categoryMatches := make(map[MarkerCategory]int, 4)
	for _, h := range hits {
		markerIDs = append(markerIDs, h.ID)
		totalMatches += h.Count
		categorySeen[h.Category] = true
		categoryMatches[h.Category] += h.Count
	}

	score := offenseScoreWeighted(hits, r.categoryPolicy)
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
	var thresholdLog *string
	var thresholdCancel func(error)
	var thresholdErr error

	r.mu.Lock()
	r.total++
	if res.Retries > 0 {
		r.retried++
		r.retries += res.Retries
	}

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
	for c, n := range categoryMatches {
		r.categoryMatchCounts[c] += n
	}

	for c := range categorySeen {
		if p, ok := r.categoryPolicy[c]; ok {
			if p.Severity > r.maxSeverity {
				r.maxSeverity = p.Severity
			}
			if p.ElevateAfterResponses > 0 && r.categoryRespCounts[c] >= p.ElevateAfterResponses && !r.elevated[c] {
				r.elevated[c] = true
				if p.ElevateTo > r.maxSeverity {
					r.maxSeverity = p.ElevateTo
				}
				s := fmt.Sprintf(
					"%s: category=%s responses=%d elevate_to=%s",
					styledKey("severity_elevated", ansiYellow, ansiBold),
					styledValue(c.String(), ansiCyan, ansiBold),
					r.categoryRespCounts[c],
					styledValue(p.ElevateTo.String(), ansiYellow, ansiBold),
				)
				thresholdLog = &s
			}
		}
	}

	if r.stopErr == nil {
		for c, p := range r.categoryPolicy {
			if p.StopAfterResponses > 0 && r.categoryRespCounts[c] >= p.StopAfterResponses {
				r.stopErr = fmt.Errorf("threshold exceeded: category %s responses %d >= %d", c, r.categoryRespCounts[c], p.StopAfterResponses)
				break
			}
			if p.StopAfterMatches > 0 && r.categoryMatchCounts[c] >= p.StopAfterMatches {
				r.stopErr = fmt.Errorf("threshold exceeded: category %s matches %d >= %d", c, r.categoryMatchCounts[c], p.StopAfterMatches)
				break
			}
		}
		if r.stopErr != nil && r.cancel != nil {
			thresholdCancel = r.cancel
			thresholdErr = r.stopErr
			s := fmt.Sprintf("%s: %s", styledKey("stop", ansiRed, ansiBold), styledValue(r.stopErr.Error(), ansiRed))
			thresholdLog = &s
		}
	}

	if offender != nil {
		r.maybeAddTopLocked(*offender)
	}

	if r.total%progressEveryN == 0 {
		s := fmt.Sprintf(
			"%s: sent=%d last_status=%s last_latency=%s",
			styledKey("progress", ansiCyan, ansiBold),
			r.total,
			styledStatusCode(res.StatusCode),
			styledValue(res.Latency.String(), ansiBlue),
		)
		progressLog = &s
	}
	r.mu.Unlock()

	if progressLog != nil {
		log.Print(*progressLog)
	}
	if thresholdLog != nil {
		log.Print(*thresholdLog)
	}
	if thresholdCancel != nil && thresholdErr != nil {
		thresholdCancel(thresholdErr)
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

	log.Printf("%s: sent=%d errs=%d", styledKey("done", ansiGreen, ansiBold), r.total, r.errs)
	log.Printf("%s: %s", styledKey("severity", ansiYellow, ansiBold), styledValue(r.maxSeverity.String(), ansiYellow, ansiBold))
	if r.retried > 0 {
		log.Printf("%s: requests=%d retries=%d", styledKey("retried", ansiYellow, ansiBold), r.retried, r.retries)
	}
	if r.firstErr != nil {
		log.Printf("%s: %v", styledKey("first_error", ansiRed, ansiBold), r.firstErr)
	}

	if r.latencyCount > 0 {
		avg := time.Duration(int64(r.latencyTotal) / int64(r.latencyCount))
		log.Printf(
			"%s: min=%s avg=%s max=%s",
			styledKey("latency", ansiBlue, ansiBold),
			styledValue(r.latencyMin.String(), ansiBlue),
			styledValue(avg.String(), ansiBlue),
			styledValue(r.latencyMax.String(), ansiBlue),
		)
	}

	if len(r.byStatus) > 0 {
		var codes []int
		for code := range r.byStatus {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		for _, code := range codes {
			log.Printf("%s: %d", styledStatusKey(code), r.byStatus[code])
		}
	}

	if len(r.categoryRespCounts) > 0 {
		var cats []string
		for c := range r.categoryRespCounts {
			cats = append(cats, string(c))
		}
		sort.Strings(cats)
		for _, c := range cats {
			log.Printf("%s: %d", styledCategoryKey(MarkerCategory(c)), r.categoryRespCounts[MarkerCategory(c)])
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
		log.Printf("%s: (responses / matches)", styledKey("markers", ansiCyan, ansiBold))
		for _, row := range rows {
			log.Printf("%s: %d / %d", styledMarkerKey(row.id), row.responses, row.matches)
		}
	}

	if len(r.top) > 0 {
		log.Printf("%s:", styledKey("top_offenders", ansiMagenta, ansiBold))
		for i, off := range r.top {
			ids := strings.Join(off.MarkerIDs, ",")
			if ids == "" {
				ids = "-"
			}
			line := fmt.Sprintf(
				"%s score=%s status=%s latency=%s markers=%s",
				styledValue("#"+intToString(i+1), ansiMagenta, ansiBold),
				styledValue(intToString(off.Score), ansiYellow, ansiBold),
				styledStatusCode(off.StatusCode),
				styledValue(off.Latency.String(), ansiBlue),
				styledValue(ids, ansiCyan),
			)
			if off.Error != "" {
				line += " " + styledKey("err", ansiRed, ansiBold) + "=" + previewOneLine(off.Error, 140)
			}
			log.Print(line)
			if off.PromptPreview != "" {
				log.Printf("%s%q", styledDetailPrefix("  prompt="), off.PromptPreview)
			}
			if off.ResponsePreview != "" {
				log.Printf("%s%q", styledDetailPrefix("  resp="), off.ResponsePreview)
			}
		}
	}
}

func offenseScoreWeighted(hits []MarkerHit, policy map[MarkerCategory]categoryPolicy) int {
	if len(hits) == 0 {
		return 0
	}
	distinctMarkers := 0
	totalMatches := 0
	weightedMatches := 0
	for _, h := range hits {
		if h.Count <= 0 {
			continue
		}
		distinctMarkers++
		totalMatches += h.Count
		w := 1
		if p, ok := policy[h.Category]; ok && p.ScoreWeight > 0 {
			w = p.ScoreWeight
		}
		weightedMatches += h.Count * w
	}
	if distinctMarkers == 0 || totalMatches == 0 {
		return 0
	}
	// Favor responses that trip many marker types even if each only matches once; amplify by category weights.
	return distinctMarkers*2 + weightedMatches
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

func (r *report) ThresholdError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopErr
}
