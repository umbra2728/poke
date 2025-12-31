package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMarkerConfigFile_CategoriesOnlyFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "markers.json")
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "categories": {
    "system_leak": { "stop_after_responses": 1 }
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := loadMarkerConfigFile(path)
	if err != nil {
		t.Fatalf("loadMarkerConfigFile: %v", err)
	}
	if len(cfg.RegexMarkers) == 0 {
		t.Fatalf("expected default regex markers, got none")
	}
	p, ok := cfg.Categories[CategorySystemLeak]
	if !ok {
		t.Fatalf("expected policy for %s", CategorySystemLeak)
	}
	if p.StopAfterResponses != 1 {
		t.Fatalf("expected StopAfterResponses=1, got %d", p.StopAfterResponses)
	}
}

func TestLoadMarkerConfigFile_ElevateRequiresElevateTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "markers.json")
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "categories": {
    "pii_leak": { "elevate_after_responses": 5 }
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := loadMarkerConfigFile(path); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestReport_StopsOnCategoryThreshold(t *testing.T) {
	cfg := defaultMarkerConfig()
	a, err := newResponseAnalyzer(cfg)
	if err != nil {
		t.Fatalf("newResponseAnalyzer: %v", err)
	}

	p := cfg.Categories[CategoryPIILeak]
	p.StopAfterResponses = 1
	cfg.Categories[CategoryPIILeak] = p

	var canceledErr error
	r := newReport(a, cfg.Categories, func(err error) { canceledErr = err }, nil)

	r.RecordResult(RequestResult{
		StatusCode: 200,
		Latency:    1 * time.Millisecond,
		Body:       []byte("Contact me at test@example.com"),
	})

	if err := r.ThresholdError(); err == nil {
		t.Fatalf("expected threshold error, got nil")
	}
	if canceledErr == nil {
		t.Fatalf("expected cancel cause to be set")
	}
}
