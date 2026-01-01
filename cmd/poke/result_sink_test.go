package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResultSink_JSONLAndCSV(t *testing.T) {
	dir := t.TempDir()
	jsonlOut := filepath.Join(dir, "out.jsonl")
	csvOut := filepath.Join(dir, "out.csv")

	s, err := newResultSink(jsonlOut, csvOut)
	if err != nil {
		t.Fatalf("newResultSink: %v", err)
	}

	s.Write(requestEvent{
		Time:        time.Unix(0, 0).UTC(),
		Seq:         1,
		WorkerID:    2,
		Prompt:      "hello",
		Attempts:    1,
		Retries:     0,
		StatusCode:  200,
		Latency:     123 * time.Millisecond,
		BodyLen:     2,
		BodyPreview: "ok",
		Error:       "",
		MarkerHits:  []MarkerHit{{ID: "m1", Category: CategorySystemLeak, Count: 2}},
		Score:       9,
		Severity:    severityError,
	})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// JSONL: validate first line is parseable and has expected fields.
	fj, err := os.Open(jsonlOut)
	if err != nil {
		t.Fatalf("Open jsonl: %v", err)
	}
	defer fj.Close()
	sc := bufio.NewScanner(fj)
	if !sc.Scan() {
		t.Fatalf("expected jsonl row")
	}
	var row map[string]any
	if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
		t.Fatalf("json.Unmarshal: %v (line=%q)", err, string(sc.Bytes()))
	}
	if row["prompt"] != "hello" || int(row["seq"].(float64)) != 1 {
		t.Fatalf("unexpected json row: %#v", row)
	}

	// CSV: validate header + one record and that marker_hits is rendered.
	fc, err := os.Open(csvOut)
	if err != nil {
		t.Fatalf("Open csv: %v", err)
	}
	defer fc.Close()
	cr := csv.NewReader(fc)
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rows (header+1), got %d", len(records))
	}
	header := records[0]
	if len(header) == 0 || header[0] != "time" {
		t.Fatalf("unexpected header: %#v", header)
	}
	rec := records[1]
	foundMarkerHits := false
	for _, col := range rec {
		if col == "m1=2" {
			foundMarkerHits = true
			break
		}
	}
	if !foundMarkerHits {
		t.Fatalf("expected marker hits in record: %#v", rec)
	}
}
