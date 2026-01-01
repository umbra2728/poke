package main

import (
	"os"
	"testing"
)

func TestUI_ColorHelpers(t *testing.T) {
	if got := trueColor(1, 2, 3); got != "\x1b[38;2;1;2;3m" {
		t.Fatalf("trueColor=%q", got)
	}
	if got := paint(false, "x", ansiRed); got != "x" {
		t.Fatalf("paint disabled=%q", got)
	}
	if got := paint(true, "", ansiRed); got != "" {
		t.Fatalf("paint empty=%q", got)
	}
	if got := paint(true, "x"); got != "x" {
		t.Fatalf("paint no codes=%q", got)
	}
	if got := paint(true, "x", ansiRed); got != ansiRed+"x"+ansiReset {
		t.Fatalf("paint enabled=%q", got)
	}

	origNoColor := os.Getenv("NO_COLOR")
	origCliColor := os.Getenv("CLICOLOR")
	origTerm := os.Getenv("TERM")
	t.Cleanup(func() { _ = os.Setenv("NO_COLOR", origNoColor) })
	t.Cleanup(func() { _ = os.Setenv("CLICOLOR", origCliColor) })
	t.Cleanup(func() { _ = os.Setenv("TERM", origTerm) })
	_ = os.Setenv("NO_COLOR", "1")
	if shouldUseColor(os.Stderr) {
		t.Fatalf("expected NO_COLOR to disable color")
	}
	_ = os.Setenv("NO_COLOR", "")
	_ = os.Setenv("CLICOLOR", "0")
	if shouldUseColor(os.Stderr) {
		t.Fatalf("expected CLICOLOR=0 to disable color")
	}
	_ = os.Setenv("CLICOLOR", "")
	_ = os.Setenv("TERM", "dumb")
	if shouldUseColor(os.Stderr) {
		t.Fatalf("expected TERM=dumb to disable color")
	}

	if isTerminal(nil) {
		t.Fatalf("expected nil file not to be terminal")
	}
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = f.Close()
	if isTerminal(f) {
		t.Fatalf("expected closed file not to be terminal")
	}

	_ = helpError{usage: "x"}.Error()
}
