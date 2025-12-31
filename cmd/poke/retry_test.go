package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendOne_RetriesOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := config{
		targetURL: srv.URL,
		method:    http.MethodPost,
		timeout:   5 * time.Second,
		retry: retryConfig{
			MaxRetries: 3,
			BackoffMin: 0,
			BackoffMax: 0,
		},
	}

	res := sendOne(t.Context(), srv.Client(), cfg, nil, nil, 1, "hi")
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if res.Attempts != 2 || res.Retries != 1 {
		t.Fatalf("expected attempts=2 retries=1, got attempts=%d retries=%d", res.Attempts, res.Retries)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestSendOne_StopsAfterMaxRetries(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	cfg := config{
		targetURL: srv.URL,
		method:    http.MethodPost,
		timeout:   5 * time.Second,
		retry: retryConfig{
			MaxRetries: 2,
			BackoffMin: 0,
			BackoffMax: 0,
		},
	}

	res := sendOne(t.Context(), srv.Client(), cfg, nil, nil, 1, "hi")
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", res.StatusCode)
	}
	if res.Attempts != 3 || res.Retries != 2 {
		t.Fatalf("expected attempts=3 retries=2, got attempts=%d retries=%d", res.Attempts, res.Retries)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendOne_RetriesOnTransportError(t *testing.T) {
	var calls int
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("dial tcp: connect: refused")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(stringsReader("ok")),
				Request:    r,
			}, nil
		}),
		Timeout: 5 * time.Second,
	}

	cfg := config{
		targetURL: "http://example.invalid/test",
		method:    http.MethodGet,
		timeout:   5 * time.Second,
		retry: retryConfig{
			MaxRetries: 1,
			BackoffMin: 0,
			BackoffMax: 0,
		},
	}

	res := sendOne(t.Context(), client, cfg, nil, nil, 1, "hi")
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if res.Attempts != 2 || res.Retries != 1 {
		t.Fatalf("expected attempts=2 retries=1, got attempts=%d retries=%d", res.Attempts, res.Retries)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func stringsReader(s string) *stringReader { return &stringReader{s: s} }

type stringReader struct{ s string }

func (r *stringReader) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}
