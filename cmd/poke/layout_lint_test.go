package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepoHasSingleMainPackage(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	modulePath := runGo(t, repoRoot, "list", "-m", "-f", "{{.Path}}")
	expectedMain := modulePath + "/cmd/poke"

	out := runGo(t, repoRoot, "list", "-f", `{{if eq .Name "main"}}{{.ImportPath}}{{"\n"}}{{end}}`, "./...")
	var mains []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			mains = append(mains, line)
		}
	}

	if len(mains) != 1 || mains[0] != expectedMain {
		t.Fatalf("expected exactly one main package (%q); found: %v", expectedMain, mains)
	}
}

func runGo(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return strings.TrimSpace(out.String())
}

