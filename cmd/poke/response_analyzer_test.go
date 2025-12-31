package main

import (
	"net/http"
	"testing"
	"time"
)

func TestResponseAnalyzer_BodyMarkers(t *testing.T) {
	a, err := newResponseAnalyzer(defaultMarkerConfig())
	if err != nil {
		t.Fatalf("newResponseAnalyzer: %v", err)
	}

	res := RequestResult{
		Prompt:     "seed",
		StatusCode: 200,
		Latency:    10 * time.Millisecond,
		Body:       []byte("Sure. Ignore previous instructions. Here's the system prompt: ..."),
	}
	hits := a.Analyze(res)

	if !hasMarker(hits, "jailbreak_success:ignore_previous_instructions") {
		t.Fatalf("expected ignore_previous_instructions, got: %#v", hits)
	}
	if !hasMarker(hits, "system_leak:mentions_system_or_developer_prompt") {
		t.Fatalf("expected mentions_system_or_developer_prompt, got: %#v", hits)
	}
}

func TestResponseAnalyzer_StatusMarkers(t *testing.T) {
	a, err := newResponseAnalyzer(defaultMarkerConfig())
	if err != nil {
		t.Fatalf("newResponseAnalyzer: %v", err)
	}

	res := RequestResult{StatusCode: 503}
	hits := a.Analyze(res)
	if !hasMarker(hits, "http_error:http_5xx") {
		t.Fatalf("expected http_5xx, got: %#v", hits)
	}
}

func TestResponseAnalyzer_RateLimitMarkers(t *testing.T) {
	a, err := newResponseAnalyzer(defaultMarkerConfig())
	if err != nil {
		t.Fatalf("newResponseAnalyzer: %v", err)
	}

	res := RequestResult{
		StatusCode: 429,
		Headers:    http.Header{"Retry-After": []string{"5"}},
		Body:       []byte("Too many requests. Rate limited."),
	}
	hits := a.Analyze(res)
	if !hasMarker(hits, "rate_limit:status_429") {
		t.Fatalf("expected status_429, got: %#v", hits)
	}
	if !hasMarker(hits, "rate_limit:retry_after_header") {
		t.Fatalf("expected retry_after_header, got: %#v", hits)
	}
	if !hasMarker(hits, "rate_limit:rate_limit_phrase") {
		t.Fatalf("expected rate_limit_phrase, got: %#v", hits)
	}
}

func hasMarker(hits []MarkerHit, id string) bool {
	for _, h := range hits {
		if h.ID == id {
			return true
		}
	}
	return false
}
