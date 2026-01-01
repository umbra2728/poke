package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	if d, ok := parseRetryAfter("", now); ok || d != 0 {
		t.Fatalf("expected empty to be false")
	}
	if d, ok := parseRetryAfter("0", now); ok || d != 0 {
		t.Fatalf("expected 0 seconds to be false")
	}
	if d, ok := parseRetryAfter("3", now); !ok || d != 3*time.Second {
		t.Fatalf("expected 3s, got %v ok=%v", d, ok)
	}

	date := now.Add(2 * time.Second).UTC().Format(http.TimeFormat)
	if d, ok := parseRetryAfter(date, now); !ok || d != 2*time.Second {
		t.Fatalf("expected 2s, got %v ok=%v", d, ok)
	}
	if _, ok := parseRetryAfter("not-a-date", now); ok {
		t.Fatalf("expected invalid to be false")
	}
}

func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(context.Background(), 0); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, 10*time.Second); err == nil {
		t.Fatalf("expected canceled error")
	}
}
