package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMarkerConfigFile_ReplaceDefaultsRequiresRegex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "markers.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"replace_defaults":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadMarkerConfigFile(path); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadMarkerConfigFile_ReplaceDefaultsRegexOnlyGetsBaselineCategories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "markers.json")
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "replace_defaults": true,
  "regexes": [
    {"id":"x","category":"system_leak","pattern":"x"}
  ]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := loadMarkerConfigFile(path)
	if err != nil {
		t.Fatalf("loadMarkerConfigFile: %v", err)
	}
	if len(cfg.RegexMarkers) != 1 {
		t.Fatalf("expected 1 regex, got %d", len(cfg.RegexMarkers))
	}
	if len(cfg.Categories) == 0 {
		t.Fatalf("expected baseline categories")
	}
}

func TestLoadMarkerConfigFile_DisableExistingMarkerWithoutPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "markers.json")
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "regexes": [
    {"id":"rate_limit_phrase","category":"rate_limit","enabled":false}
  ]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := loadMarkerConfigFile(path)
	if err != nil {
		t.Fatalf("loadMarkerConfigFile: %v", err)
	}
	// Marker exists in defaults; should now be disabled.
	found := false
	for _, rm := range cfg.RegexMarkers {
		if rm.Category == CategoryRateLimit && rm.ID == "rate_limit_phrase" {
			found = true
			if rm.Enabled {
				t.Fatalf("expected marker to be disabled")
			}
		}
	}
	if !found {
		t.Fatalf("expected default marker to be present")
	}
}

func TestLoadMarkerConfigFile_RegexValidationErrors(t *testing.T) {
	dir := t.TempDir()

	unsupported := filepath.Join(dir, "v2.json")
	if err := os.WriteFile(unsupported, []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadMarkerConfigFile(unsupported); err == nil {
		t.Fatalf("expected error")
	}

	dup := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(dup, []byte(`{
  "version": 1,
  "regexes": [
    {"id":"x","category":"system_leak","pattern":"x"},
    {"id":"x","category":"system_leak","pattern":"y"}
  ]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadMarkerConfigFile(dup); err == nil {
		t.Fatalf("expected error")
	}

	missingID := filepath.Join(dir, "missing_id.json")
	if err := os.WriteFile(missingID, []byte(`{
  "version": 1,
  "regexes": [{"id":"","category":"system_leak","pattern":"x"}]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadMarkerConfigFile(missingID); err == nil {
		t.Fatalf("expected error")
	}

	missingPattern := filepath.Join(dir, "missing_pattern.json")
	if err := os.WriteFile(missingPattern, []byte(`{
  "version": 1,
  "regexes": [{"id":"new","category":"system_leak","pattern":""}]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadMarkerConfigFile(missingPattern); err == nil {
		t.Fatalf("expected error")
	}
}
