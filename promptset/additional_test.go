package promptset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestOpenPath_StdinAndMissing(t *testing.T) {
	if r, closeFn, err := openPath("-"); err != nil || r == nil || closeFn != nil {
		t.Fatalf("openPath(-): r_nil=%v closeFn_nil=%v err=%v", r == nil, closeFn == nil, err)
	}
	if _, _, err := openPath("/definitely/does/not/exist.txt"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStream_StdinText(t *testing.T) {
	orig := os.Stdin
	t.Cleanup(func() { os.Stdin = orig })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdin = r
	_, _ = w.WriteString("\n# c\nhi\n")
	_ = w.Close()

	ctx := context.Background()
	outCh := make(chan string, 8)
	err = Stream(ctx, "-", outCh, Options{})
	close(outCh)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []string
	for p := range outCh {
		got = append(got, p)
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("unexpected prompts: %#v", got)
	}
}

func TestStream_JSON_Errors(t *testing.T) {
	ctx := context.Background()
	outCh := make(chan string, 8)

	// Extra trailing content.
	if err := streamJSON(ctx, stringsReader(t, `["a"] 123`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	// Missing prompts key.
	if err := streamJSON(ctx, stringsReader(t, `{"x":1}`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	// Empty prompt.
	if err := streamJSON(ctx, stringsReader(t, `[" "]`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	// Non-string prompt.
	if err := streamJSON(ctx, stringsReader(t, `[{"prompt":1}]`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStream_JSONL_Errors(t *testing.T) {
	ctx := context.Background()
	outCh := make(chan string, 8)

	if err := streamJSONL(ctx, stringsReader(t, `not-json`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	if err := streamJSONL(ctx, stringsReader(t, `"unterminated`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	if err := streamJSONL(ctx, stringsReader(t, `""`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	if err := streamJSONL(ctx, stringsReader(t, `{"prompt":" ","disabled":false}`), outCh, Options{}); err == nil {
		t.Fatalf("expected error")
	}
	if err := streamJSONL(ctx, stringsReader(t, `{"prompt":"x","disabled":true}`), outCh, Options{}); err != nil {
		t.Fatalf("expected nil for disabled prompt, got %v", err)
	}
}

func TestEmitPrompt_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outCh := make(chan string)

	if err := send(ctx, outCh, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestParsePromptJSON_Errors(t *testing.T) {
	if _, err := parsePromptJSON(123); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parsePromptJSON(map[string]any{"prompts": 1}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parsePromptJSONArray([]any{map[string]any{"x": 1}}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parsePromptJSONArray([]any{map[string]any{"prompt": 1}}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parsePromptJSONArray([]any{1}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStreamText_ReadError(t *testing.T) {
	ctx := context.Background()
	outCh := make(chan string, 8)
	err := streamText(ctx, &failingReader{data: []byte("a\n"), err: io.ErrUnexpectedEOF}, outCh, Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestStreamJSONL_ReadError(t *testing.T) {
	ctx := context.Background()
	outCh := make(chan string, 8)
	err := streamJSONL(ctx, &failingReader{data: []byte("\"a\"\n"), err: fmt.Errorf("boom")}, outCh, Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
}

type failingReader struct {
	data []byte
	err  error
	done bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	n := copy(p, r.data)
	r.data = r.data[n:]
	if n == 0 {
		return 0, r.err
	}
	return n, nil
}

func stringsReader(t *testing.T, s string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "in.*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
