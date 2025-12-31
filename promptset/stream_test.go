package promptset

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func collectStream(t *testing.T, path string, opt Options) ([]string, error) {
	t.Helper()
	ctx := context.Background()
	outCh := make(chan string, 128)
	err := Stream(ctx, path, outCh, opt)
	close(outCh)
	var got []string
	for p := range outCh {
		got = append(got, p)
	}
	return got, err
}

func TestStream_Text(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.txt")
	if err := os.WriteFile(path, []byte("\n# comment\nhello\n world \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := collectStream(t, path, Options{})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	want := []string{"hello", "world"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestStream_JSON_Array(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	if err := os.WriteFile(path, []byte(`["a", {"prompt":"b"}, {"prompt":"c","disabled":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := collectStream(t, path, Options{})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestStream_JSON_ObjectWithPrompts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"prompts":["x",{"prompt":"y"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := collectStream(t, path, Options{})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	want := []string{"x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestStream_JSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.jsonl")
	if err := os.WriteFile(path, []byte("\"a\"\n{\"prompt\":\"b\"}\n# ok\n{\"prompt\":\"c\",\"disabled\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := collectStream(t, path, Options{})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
