package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequestTemplate_FromFiles(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "body.json")
	if err := os.WriteFile(bodyPath, []byte(`{"prompt":"{{prompt}}"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	queryPath := filepath.Join(dir, "query.txt")
	if err := os.WriteFile(queryPath, []byte(`prompt={{prompt}}&model=x`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := config{
		targetURL:     "https://example.test/api",
		method:        http.MethodPost,
		bodyTmplFile:  bodyPath,
		queryTmplFile: queryPath,
	}
	tmpl, err := loadRequestTemplate(cfg)
	if err != nil {
		t.Fatalf("loadRequestTemplate: %v", err)
	}
	if tmpl.body == nil || tmpl.query == nil {
		t.Fatalf("expected both body and query templates")
	}
}

func TestLoadTemplateText_FileEmptyRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(p, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadTemplateText("", p, "x"); err == nil {
		t.Fatalf("expected error")
	}
}
