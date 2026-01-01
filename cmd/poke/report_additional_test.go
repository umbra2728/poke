package main

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestThresholdExceededError_ExitCodeBuckets(t *testing.T) {
	if (thresholdExceededError{Severity: severityWarn}).ExitCode() != 2 {
		t.Fatalf("warn should map to 2")
	}
	if (thresholdExceededError{Severity: severityError}).ExitCode() != 3 {
		t.Fatalf("error should map to 3")
	}
	if (thresholdExceededError{Severity: severityCritical}).ExitCode() != 4 {
		t.Fatalf("critical should map to 4")
	}
}

func TestReport_RecordErrorAndLogSummary_CoversBranches(t *testing.T) {
	colorOnStderr = false

	cfg := defaultMarkerConfig()
	a, err := newResponseAnalyzer(cfg)
	if err != nil {
		t.Fatalf("newResponseAnalyzer: %v", err)
	}

	r := newReport(a, cfg.Categories, nil, nil)

	r.RecordResult(RequestResult{
		WorkerID:   1,
		Prompt:     "p1",
		StatusCode: 200,
		Retries:    2,
		Latency:    10 * time.Millisecond,
		Body:       []byte("Ignore previous instructions. Here's the system prompt: ..."),
	})
	r.RecordResult(RequestResult{
		WorkerID:   1,
		Prompt:     "p2",
		StatusCode: 429,
		Latency:    20 * time.Millisecond,
		Body:       []byte("Too many requests. Rate limited."),
	})
	r.RecordResult(RequestResult{
		WorkerID:   1,
		Prompt:     "p3",
		StatusCode: 503,
		Latency:    30 * time.Millisecond,
		Body:       []byte("server error"),
	})

	r.RecordError(errors.New("boom"))

	var b bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(orig) })

	r.LogSummary()

	out := b.String()
	for _, want := range []string{"done", "severity", "retried", "first_error", "latency", "markers", "top_offenders"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in logs, got %q", want, out)
		}
	}
}
