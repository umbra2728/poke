package main

import "testing"

func TestRetryConfig_Validate(t *testing.T) {
	if err := (retryConfig{MaxRetries: -1}).validate(); err == nil {
		t.Fatalf("expected error")
	}
	if err := (retryConfig{MaxRetries: 1, BackoffMin: -1}).validate(); err == nil {
		t.Fatalf("expected error")
	}
	if err := (retryConfig{MaxRetries: 1, BackoffMin: 2, BackoffMax: -1}).validate(); err == nil {
		t.Fatalf("expected error")
	}
	if err := (retryConfig{MaxRetries: 1, BackoffMin: 2, BackoffMax: 1}).validate(); err == nil {
		t.Fatalf("expected error")
	}
	if err := (retryConfig{MaxRetries: 0, BackoffMin: 0, BackoffMax: 0}).validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestSeverityLevel_StringAndParse(t *testing.T) {
	if severityInfo.String() != "info" || severityWarn.String() != "warn" || severityError.String() != "error" || severityCritical.String() != "critical" {
		t.Fatalf("unexpected severity strings")
	}
	if got, err := parseSeverityLevel("warning"); err != nil || got != severityWarn {
		t.Fatalf("parseSeverityLevel(warning)=%v,%v", got, err)
	}
	if _, err := parseSeverityLevel("wat"); err == nil {
		t.Fatalf("expected error")
	}
}
