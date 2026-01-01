package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestReadResponseBodyExact_TruncatesAndFlags(t *testing.T) {
	resp := &http.Response{
		Body:          io.NopCloser(bytes.NewReader([]byte("0123456789"))),
		ContentLength: 10,
	}
	b, truncated, err := readResponseBody(resp, 5, false)
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if string(b) != "01234" {
		t.Fatalf("unexpected body: %q", string(b))
	}
	if !truncated {
		t.Fatalf("expected truncated=true")
	}
}

func TestReadResponseBodyStream_UsesContentLengthWhenAvailable(t *testing.T) {
	resp := &http.Response{
		Body:          io.NopCloser(bytes.NewReader([]byte("hello"))),
		ContentLength: 5,
	}
	b, truncated, err := readResponseBody(resp, 5, true)
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected body: %q", string(b))
	}
	if truncated {
		t.Fatalf("expected truncated=false")
	}
}

func TestReadResponseBodyStream_UnknownLengthIsConservative(t *testing.T) {
	resp := &http.Response{
		Body:          io.NopCloser(bytes.NewReader([]byte("hello"))),
		ContentLength: -1,
	}
	b, truncated, err := readResponseBody(resp, 5, true)
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected body: %q", string(b))
	}
	if !truncated {
		t.Fatalf("expected truncated=true")
	}
}
