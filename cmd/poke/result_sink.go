package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type requestEvent struct {
	Time       time.Time
	Seq        int
	WorkerID   int
	Prompt     string
	Attempts   int
	Retries    int
	StatusCode int
	Latency    time.Duration
	BodyLen    int
	BodyPreview string
	Error      string

	MarkerHits []MarkerHit
	Score      int
	Severity   severityLevel
}

type resultWriter interface {
	Write(e requestEvent) error
	Close() error
}

type multiResultWriter struct {
	ws []resultWriter
}

func (m multiResultWriter) Write(e requestEvent) error {
	for _, w := range m.ws {
		if err := w.Write(e); err != nil {
			return err
		}
	}
	return nil
}

func (m multiResultWriter) Close() error {
	var first error
	for _, w := range m.ws {
		if err := w.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type jsonlWriter struct {
	f  *os.File
	bw *bufio.Writer
}

func newJSONLWriter(path string) (*jsonlWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create -jsonl-out: %w", err)
	}
	return &jsonlWriter{f: f, bw: bufio.NewWriterSize(f, 256*1024)}, nil
}

type jsonlRow struct {
	Time       string      `json:"time"`
	Seq        int         `json:"seq"`
	WorkerID   int         `json:"worker_id"`
	Prompt     string      `json:"prompt"`
	Attempts   int         `json:"attempts"`
	Retries    int         `json:"retries"`
	StatusCode int         `json:"status_code"`
	LatencyMS  int64       `json:"latency_ms"`
	BodyLen    int         `json:"body_len"`
	BodyPreview string     `json:"body_preview,omitempty"`
	Error      string      `json:"error,omitempty"`
	MarkerHits []MarkerHit `json:"marker_hits,omitempty"`
	Score      int         `json:"score"`
	Severity   string      `json:"severity"`
}

func (w *jsonlWriter) Write(e requestEvent) error {
	row := jsonlRow{
		Time:       e.Time.UTC().Format(time.RFC3339Nano),
		Seq:        e.Seq,
		WorkerID:   e.WorkerID,
		Prompt:     e.Prompt,
		Attempts:   e.Attempts,
		Retries:    e.Retries,
		StatusCode: e.StatusCode,
		LatencyMS:  e.Latency.Milliseconds(),
		BodyLen:    e.BodyLen,
		Error:      e.Error,
		MarkerHits: e.MarkerHits,
		Score:      e.Score,
		Severity:   e.Severity.String(),
		BodyPreview: e.BodyPreview,
	}

	b, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("encode jsonl row: %w", err)
	}
	if _, err := w.bw.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write jsonl: %w", err)
	}
	return nil
}

func (w *jsonlWriter) Close() error {
	if w == nil {
		return nil
	}
	var first error
	if w.bw != nil {
		if err := w.bw.Flush(); err != nil && first == nil {
			first = err
		}
	}
	if w.f != nil {
		if err := w.f.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type csvWriter struct {
	f  *os.File
	bw *bufio.Writer
	w  *csv.Writer
}

func newCSVWriter(path string) (*csvWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create -csv-out: %w", err)
	}
	bw := bufio.NewWriterSize(f, 256*1024)
	w := csv.NewWriter(bw)
	// Stable columns to keep it easy to ingest.
	if err := w.Write([]string{
		"time",
		"seq",
		"worker_id",
		"attempts",
		"retries",
		"status_code",
		"latency_ms",
		"body_len",
		"severity",
		"score",
		"marker_hits",
		"error",
		"prompt",
		"body_preview",
	}); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write csv header: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flush csv header: %w", err)
	}
	return &csvWriter{f: f, bw: bw, w: w}, nil
}

func markerHitsCSV(hits []MarkerHit) string {
	if len(hits) == 0 {
		return ""
	}
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		parts = append(parts, h.ID+"="+intToString(h.Count))
	}
	return strings.Join(parts, ";")
}

func (w *csvWriter) Write(e requestEvent) error {
	rec := []string{
		e.Time.UTC().Format(time.RFC3339Nano),
		intToString(e.Seq),
		intToString(e.WorkerID),
		intToString(e.Attempts),
		intToString(e.Retries),
		intToString(e.StatusCode),
		intToString(int(e.Latency.Milliseconds())),
		intToString(e.BodyLen),
		e.Severity.String(),
		intToString(e.Score),
		markerHitsCSV(e.MarkerHits),
		e.Error,
		e.Prompt,
		e.BodyPreview,
	}
	if err := w.w.Write(rec); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	return nil
}

func (w *csvWriter) Close() error {
	if w == nil {
		return nil
	}
	var first error
	if w.w != nil {
		w.w.Flush()
		if err := w.w.Error(); err != nil && first == nil {
			first = err
		}
	}
	if w.bw != nil {
		if err := w.bw.Flush(); err != nil && first == nil {
			first = err
		}
	}
	if w.f != nil {
		if err := w.f.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type resultSink struct {
	ch        chan requestEvent
	done      chan struct{}
	closeOnce sync.Once

	mu  sync.Mutex
	err error

	w resultWriter
}

func newResultSink(jsonlOut, csvOut string) (*resultSink, error) {
	if jsonlOut == "" && csvOut == "" {
		return nil, nil
	}
	var writers []resultWriter
	if jsonlOut != "" {
		w, err := newJSONLWriter(jsonlOut)
		if err != nil {
			return nil, err
		}
		writers = append(writers, w)
	}
	if csvOut != "" {
		w, err := newCSVWriter(csvOut)
		if err != nil {
			for _, ww := range writers {
				_ = ww.Close()
			}
			return nil, err
		}
		writers = append(writers, w)
	}

	s := &resultSink{
		ch:   make(chan requestEvent, 1024),
		done: make(chan struct{}),
		w:    multiResultWriter{ws: writers},
	}
	go s.loop()
	return s, nil
}

func (s *resultSink) loop() {
	defer close(s.done)
	for e := range s.ch {
		if s.hasErr() {
			continue
		}
		if err := s.w.Write(e); err != nil {
			s.setErr(err)
		}
	}
	if err := s.w.Close(); err != nil && !s.hasErr() {
		s.setErr(err)
	}
}

func (s *resultSink) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *resultSink) hasErr() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err != nil
}

func (s *resultSink) Write(e requestEvent) {
	if s == nil {
		return
	}
	if s.hasErr() {
		return
	}
	s.ch <- e
}

func (s *resultSink) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() { close(s.ch) })
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
