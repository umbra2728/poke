package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkers  = 10
	defaultTimeout  = 30 * time.Second
	maxPromptBytes  = 1 << 20  // 1 MiB
	maxBodyBytes    = 2 << 20  // 2 MiB
	progressEveryN  = 100
	defaultMethod   = "POST"
	defaultJSONKey  = "prompt"
)

type config struct {
	targetURL   string
	method      string
	headersFile string
	cookiesFile string
	workers     int
	rate        float64
	timeout     time.Duration
	promptsFile string
}

func main() {
	log.SetFlags(0)

	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		var he helpError
		if errors.As(err, &he) {
			fmt.Fprint(os.Stdout, he.usage)
			return
		}
		log.Fatalf("error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("error: %v", err)
	}
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("poke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&cfg.targetURL, "url", "", "Target URL (required)")
	fs.StringVar(&cfg.method, "method", defaultMethod, "HTTP method (GET/POST/...)")
	fs.StringVar(&cfg.headersFile, "headers-file", "", "Path to headers file (Key: Value per line); optional")
	fs.StringVar(&cfg.cookiesFile, "cookies-file", "", "Path to cookies file (name=value per line); optional")
	fs.IntVar(&cfg.workers, "workers", defaultWorkers, "Number of concurrent workers")
	fs.Float64Var(&cfg.rate, "rate", 0, "Global rate limit (requests/sec); 0 = unlimited")
	fs.DurationVar(&cfg.timeout, "timeout", defaultTimeout, "Per-request timeout (e.g. 10s, 1m)")
	fs.StringVar(&cfg.promptsFile, "prompts", "", "Prompt source file (one prompt per line); use '-' for stdin (required)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config{}, helpError{usage: usageText(fs)}
		}
		return config{}, usageError(err, fs)
	}
	if cfg.targetURL == "" || cfg.promptsFile == "" {
		return config{}, usageError(fmt.Errorf("missing required flags: -url and -prompts"), fs)
	}
	if cfg.workers <= 0 {
		return config{}, fmt.Errorf("-workers must be > 0")
	}
	if cfg.rate < 0 {
		return config{}, fmt.Errorf("-rate must be >= 0")
	}
	cfg.method = strings.ToUpper(strings.TrimSpace(cfg.method))
	if cfg.method == "" {
		return config{}, fmt.Errorf("-method must not be empty")
	}
	if _, err := url.ParseRequestURI(cfg.targetURL); err != nil {
		return config{}, fmt.Errorf("invalid -url: %w", err)
	}
	return cfg, nil
}

func usageError(cause error, fs *flag.FlagSet) error {
	return errors.New(cause.Error() + "\n\n" + usageText(fs))
}

type helpError struct {
	usage string
}

func (e helpError) Error() string { return "help requested" }

func usageText(fs *flag.FlagSet) string {
	var b strings.Builder
	b.WriteString("Usage:\n  poke -url URL -prompts FILE [flags]\n\nFlags:\n")
	fs.SetOutput(&b)
	fs.PrintDefaults()
	return b.String()
}

func run(ctx context.Context, cfg config) error {
	headers, err := readHeadersFile(cfg.headersFile)
	if err != nil {
		return err
	}
	cookies, err := readCookiesFile(cfg.cookiesFile)
	if err != nil {
		return err
	}

	limiter, err := newRateLimiter(cfg.rate)
	if err != nil {
		return err
	}
	defer limiter.Close()

	client := &http.Client{Timeout: cfg.timeout}

	prompts := make(chan string, cfg.workers*2)
	var wg sync.WaitGroup

	stats := newStats()

	wg.Add(cfg.workers)
	for i := 0; i < cfg.workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			worker(ctx, workerID, cfg, client, limiter, headers, cookies, prompts, stats)
		}(i + 1)
	}

	readErr := make(chan error, 1)
	go func() {
		defer close(prompts)
		readErr <- streamPrompts(ctx, cfg.promptsFile, prompts)
	}()

	wg.Wait()

	if err := <-readErr; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	stats.LogSummary()
	return nil
}

func streamPrompts(ctx context.Context, path string, out chan<- string) error {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open prompts file: %w", err)
		}
		defer f.Close()
		r = f
	}

	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxPromptBytes)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- line:
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read prompts: %w", err)
	}
	return nil
}

func worker(
	ctx context.Context,
	workerID int,
	cfg config,
	client *http.Client,
	limiter *rateLimiter,
	baseHeaders http.Header,
	cookies []*http.Cookie,
	in <-chan string,
	stats *stats,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case prompt, ok := <-in:
			if !ok {
				return
			}
			if err := limiter.Wait(ctx); err != nil {
				stats.RecordError(err)
				return
			}

			resp, body, err := sendOne(ctx, client, cfg, baseHeaders, cookies, prompt)
			if err != nil {
				stats.RecordError(err)
				continue
			}
			analyzePlaceholder(workerID, prompt, resp, body, stats)
		}
	}
}

func sendOne(
	ctx context.Context,
	client *http.Client,
	cfg config,
	baseHeaders http.Header,
	cookies []*http.Cookie,
	prompt string,
) (*http.Response, []byte, error) {
	u, err := url.Parse(cfg.targetURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse -url: %w", err)
	}

	var body io.Reader
	if cfg.method == http.MethodGet {
		q := u.Query()
		q.Set(defaultJSONKey, prompt)
		u.RawQuery = q.Encode()
	} else {
		payload := map[string]string{defaultJSONKey: prompt}
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal json payload: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, cfg.method, u.String(), body)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}

	for k, vs := range baseHeaders {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if cfg.method != http.MethodGet && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}
	return resp, b, nil
}

func analyzePlaceholder(workerID int, prompt string, resp *http.Response, body []byte, stats *stats) {
	stats.RecordResponse(resp.StatusCode)
	n := stats.Total()
	if n%progressEveryN == 0 {
		log.Printf("progress: sent=%d last_status=%d", n, resp.StatusCode)
	}

	// Placeholder for future analysis. Keep minimal to avoid noisy logs.
	_ = workerID
	_ = prompt
	_ = body
}

type rateLimiter struct {
	t *time.Ticker
}

func newRateLimiter(rps float64) (*rateLimiter, error) {
	if rps == 0 {
		return &rateLimiter{t: nil}, nil
	}
	if rps < 0 {
		return nil, fmt.Errorf("rate must be >= 0")
	}
	d := time.Duration(float64(time.Second) / rps)
	if d < time.Nanosecond {
		d = time.Nanosecond
	}
	return &rateLimiter{t: time.NewTicker(d)}, nil
}

func (rl *rateLimiter) Wait(ctx context.Context) error {
	if rl.t == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-rl.t.C:
		return nil
	}
}

func (rl *rateLimiter) Close() {
	if rl.t != nil {
		rl.t.Stop()
	}
}

func readHeadersFile(path string) (http.Header, error) {
	h := make(http.Header)
	if path == "" {
		return h, nil
	}
	lines, err := readLines(path, "headers")
	if err != nil {
		return nil, err
	}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("headers file: line %d: expected 'Key: Value'", i+1)
		}
		k = http.CanonicalHeaderKey(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if k == "" {
			return nil, fmt.Errorf("headers file: line %d: empty header key", i+1)
		}
		h.Add(k, v)
	}
	return h, nil
}

func readCookiesFile(path string) ([]*http.Cookie, error) {
	if path == "" {
		return nil, nil
	}
	lines, err := readLines(path, "cookies")
	if err != nil {
		return nil, err
	}
	var out []*http.Cookie
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("cookies file: line %d: expected 'name=value'", i+1)
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			return nil, fmt.Errorf("cookies file: line %d: empty cookie name", i+1)
		}
		out = append(out, &http.Cookie{Name: name, Value: value})
	}
	return out, nil
}

func readLines(path string, kind string) ([]string, error) {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s file: %w", kind, err)
		}
		defer f.Close()
		r = f
	}
	sc := bufio.NewScanner(r)
	var out []string
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s file: %w", kind, err)
	}
	return out, nil
}

type stats struct {
	mu          sync.Mutex
	total       int
	errs        int
	byStatus    map[int]int
	firstErr    error
}

func newStats() *stats {
	return &stats{byStatus: make(map[int]int)}
}

func (s *stats) RecordResponse(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total++
	s.byStatus[status]++
}

func (s *stats) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total++
	s.errs++
	if s.firstErr == nil {
		s.firstErr = err
	}
}

func (s *stats) Total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

func (s *stats) LogSummary() {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.Printf("done: sent=%d errs=%d", s.total, s.errs)
	if s.firstErr != nil {
		log.Printf("first_error: %v", s.firstErr)
	}
	for code, n := range s.byStatus {
		log.Printf("status_%d: %d", code, n)
	}
}
