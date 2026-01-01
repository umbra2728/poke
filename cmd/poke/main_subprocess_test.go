package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_Help_PrintsUsage(t *testing.T) {
	out, code := runHelperMain(t, "-h")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (out=%q)", code, out)
	}
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "Flags:") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCLI_MissingRequiredFlags_ExitsNonZero(t *testing.T) {
	out, code := runHelperMain(t, "-url=https://example.test")
	if code == 0 {
		t.Fatalf("expected non-zero exit (out=%q)", out)
	}
	if !strings.Contains(out, "missing required flags") {
		t.Fatalf("expected missing required flags error, got: %q", out)
	}
}

func runHelperMain(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestHelperMain", "--"}, args...)...)
	cmd.Env = append(os.Environ(),
		"POKE_TEST_MAIN=1",
		"POKE_NO_BANNER=1",
		"NO_COLOR=1",
		"TERM=dumb",
	)
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := cmd.Run()
	if err == nil {
		return b.String(), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return b.String(), ee.ExitCode()
	}
	t.Fatalf("unexpected run error: %v", err)
	return "", 0
}

func TestHelperMain(t *testing.T) {
	if os.Getenv("POKE_TEST_MAIN") != "1" {
		return
	}

	// Args passed after "--".
	args := []string{}
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	os.Args = append([]string{"poke"}, args...)
	main()
	os.Exit(0)
}
