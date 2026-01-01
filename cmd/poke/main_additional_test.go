package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseFlags_Help(t *testing.T) {
	_, err := parseFlags([]string{"-h"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var he helpError
	if !errors.As(err, &he) {
		t.Fatalf("expected helpError, got %T: %v", err, err)
	}
	if !strings.Contains(he.usage, "Usage:") || !strings.Contains(he.usage, "Flags:") {
		t.Fatalf("unexpected help text: %q", he.usage)
	}
}

func TestParseFlags_ValidatesRequiredAndConflicts(t *testing.T) {
	if _, err := parseFlags([]string{}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-body-template={}", "-body-template-file=y"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-query-template=a=b", "-query-template-file=y"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=not a url", "-prompts=x"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-workers=0"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-rate=-1"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-method=   "}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-jsonl-out=-"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-csv-out=-"}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseFlags([]string{"-url=https://example.test", "-prompts=x", "-jsonl-out=out", "-csv-out=out"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReadHeadersAndCookiesFile(t *testing.T) {
	dir := t.TempDir()

	hpath := filepath.Join(dir, "headers.txt")
	if err := os.WriteFile(hpath, []byte(`
# comment
X-Test:  a
X-Test: b
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	h, err := readHeadersFile(hpath)
	if err != nil {
		t.Fatalf("readHeadersFile: %v", err)
	}
	if got := strings.Join(h.Values("X-Test"), ","); got != "a,b" {
		t.Fatalf("unexpected header values: %q", got)
	}

	badHeaders := filepath.Join(dir, "bad_headers.txt")
	if err := os.WriteFile(badHeaders, []byte("NoColonHere\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := readHeadersFile(badHeaders); err == nil {
		t.Fatalf("expected error")
	}

	cpath := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cpath, []byte(`
# comment
 a = 1
b=2
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs, err := readCookiesFile(cpath)
	if err != nil {
		t.Fatalf("readCookiesFile: %v", err)
	}
	if len(cs) != 2 || cs[0].Name != "a" || cs[0].Value != "1" || cs[1].Name != "b" || cs[1].Value != "2" {
		t.Fatalf("unexpected cookies: %#v", cs)
	}
}

func TestReadLines_Stdin(t *testing.T) {
	orig := os.Stdin
	t.Cleanup(func() { os.Stdin = orig })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdin = r
	_, _ = w.WriteString("a\nb\n")
	_ = w.Close()

	lines, err := readLines("-", "headers")
	if err != nil {
		t.Fatalf("readLines: %v", err)
	}
	if strings.Join(lines, ",") != "a,b" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestRateLimiter_WaitCanceled(t *testing.T) {
	rl, err := newRateLimiter(1)
	if err != nil {
		t.Fatalf("newRateLimiter: %v", err)
	}
	t.Cleanup(rl.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rl.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRun_Smoke(t *testing.T) {
	colorOnStderr = false

	var gotRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests++
		defer r.Body.Close()
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %q", ct)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	prompts := filepath.Join(dir, "prompts.txt")
	if err := os.WriteFile(prompts, []byte("p1\np2\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	headersFile := filepath.Join(dir, "headers.txt")
	if err := os.WriteFile(headersFile, []byte("X-Test: a\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cookiesFile := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookiesFile, []byte("sid=1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	jsonlOut := filepath.Join(dir, "out.jsonl")
	csvOut := filepath.Join(dir, "out.csv")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config{
		targetURL:    srv.URL,
		method:       http.MethodPost,
		headersFile:  headersFile,
		cookiesFile:  cookiesFile,
		workers:      2,
		rate:         0,
		timeout:      2 * time.Second,
		promptsFile:  prompts,
		retry:        retryConfig{MaxRetries: 0},
		jsonlOut:     jsonlOut,
		csvOut:       csvOut,
		ciExitCodes:  false,
		markersFile:  "",
		bodyTmplStr:  "",
		queryTmplStr: "",
	}

	var logs bytes.Buffer
	origOutput := logWriterSwap(t, &logs)
	defer origOutput()

	if err := run(ctx, cfg); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotRequests != 2 {
		t.Fatalf("expected 2 requests, got %d", gotRequests)
	}
	if _, err := os.Stat(jsonlOut); err != nil {
		t.Fatalf("expected jsonl output file: %v", err)
	}
	if _, err := os.Stat(csvOut); err != nil {
		t.Fatalf("expected csv output file: %v", err)
	}
	if !strings.Contains(logs.String(), "done") {
		t.Fatalf("expected summary logs, got: %q", logs.String())
	}
}

func logWriterSwap(t *testing.T, dst *bytes.Buffer) (restore func()) {
	t.Helper()
	// log package writes to stderr by default; keep output contained for tests.
	orig := log.Writer()
	log.SetOutput(dst)
	return func() { log.SetOutput(orig) }
}
