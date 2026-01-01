package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"poke/promptset"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkers          = 10
	defaultTimeout          = 30 * time.Second
	defaultMaxResponseBytes = 2 << 20 // 2 MiB
	progressEveryN          = 100
	defaultMethod           = "POST"
	defaultJSONKey          = "prompt"
)

type config struct {
	targetURL     string
	method        string
	headersFile   string
	cookiesFile   string
	markersFile   string
	bodyTmplStr   string
	bodyTmplFile  string
	queryTmplStr  string
	queryTmplFile string
	maxRespBytes  int64
	streamResp    bool
	workers       int
	rate          float64
	timeout       time.Duration
	promptsFile   string
	retry         retryConfig
	jsonlOut      string
	csvOut        string
	ciExitCodes   bool

	reqTemplate requestTemplate
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
		log.Fatalf("%s %v", styledErrorPrefix(), err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if b := bannerFor(os.Stderr); b != "" {
		log.Print(b)
	}

	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		var te thresholdExceededError
		if cfg.ciExitCodes && errors.As(err, &te) {
			log.Printf("%s %v", styledErrorPrefix(), err)
			os.Exit(te.ExitCode())
		}
		log.Fatalf("%s %v", styledErrorPrefix(), err)
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
	fs.StringVar(&cfg.markersFile, "markers-file", "", "Path to markers config JSON (regexes + per-category thresholds); optional")
	fs.StringVar(&cfg.bodyTmplStr, "body-template", "", "JSON request body template (non-GET); supports {{prompt}} placeholder")
	fs.StringVar(&cfg.bodyTmplFile, "body-template-file", "", "Path to JSON request body template file; supports {{prompt}} placeholder")
	fs.StringVar(&cfg.queryTmplStr, "query-template", "", "URL query template (k=v&k2=v2); values support {{prompt}} placeholder")
	fs.StringVar(&cfg.queryTmplFile, "query-template-file", "", "Path to URL query template file; values support {{prompt}} placeholder")
	fs.Int64Var(&cfg.maxRespBytes, "max-response-bytes", defaultMaxResponseBytes, "Max response bytes to read/store/analyze (0 = unlimited)")
	fs.BoolVar(&cfg.streamResp, "stream-response", false, "Stream response body reads and truncate at -max-response-bytes (faster; truncation may be conservative)")
	fs.IntVar(&cfg.workers, "workers", defaultWorkers, "Number of concurrent workers")
	fs.Float64Var(&cfg.rate, "rate", 0, "Global rate limit (requests/sec); 0 = unlimited")
	fs.DurationVar(&cfg.timeout, "timeout", defaultTimeout, "Per-request timeout (e.g. 10s, 1m)")
	fs.StringVar(&cfg.promptsFile, "prompts", "", "Prompt source file (.txt/.json/.jsonl); use '-' for stdin (required)")
	fs.IntVar(&cfg.retry.MaxRetries, "retries", 0, "Max retries for transport errors/429/5xx; 0 = disabled")
	fs.DurationVar(&cfg.retry.BackoffMin, "backoff-min", 200*time.Millisecond, "Min retry backoff delay")
	fs.DurationVar(&cfg.retry.BackoffMax, "backoff-max", 5*time.Second, "Max retry backoff delay; 0 = no cap")
	fs.StringVar(&cfg.jsonlOut, "jsonl-out", "", "Write per-request results to JSONL file (path); optional")
	fs.StringVar(&cfg.csvOut, "csv-out", "", "Write per-request results to CSV file (path); optional")
	fs.BoolVar(&cfg.ciExitCodes, "ci-exit-codes", false, "Use CI-friendly exit codes when marker stop thresholds trigger (2=warn/info, 3=error, 4=critical)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config{}, helpError{usage: usageText(fs)}
		}
		return config{}, usageError(err, fs)
	}
	if cfg.targetURL == "" || cfg.promptsFile == "" {
		return config{}, usageError(fmt.Errorf("missing required flags: -url and -prompts"), fs)
	}
	if cfg.bodyTmplStr != "" && cfg.bodyTmplFile != "" {
		return config{}, usageError(fmt.Errorf("only one of -body-template or -body-template-file may be set"), fs)
	}
	if cfg.queryTmplStr != "" && cfg.queryTmplFile != "" {
		return config{}, usageError(fmt.Errorf("only one of -query-template or -query-template-file may be set"), fs)
	}
	if cfg.workers <= 0 {
		return config{}, fmt.Errorf("-workers must be > 0")
	}
	if cfg.rate < 0 {
		return config{}, fmt.Errorf("-rate must be >= 0")
	}
	if cfg.maxRespBytes < 0 {
		return config{}, fmt.Errorf("-max-response-bytes must be >= 0")
	}
	if err := cfg.retry.validate(); err != nil {
		return config{}, usageError(err, fs)
	}
	if cfg.jsonlOut == "-" || cfg.csvOut == "-" {
		return config{}, fmt.Errorf("structured outputs must be file paths; '-' is not supported (keeps stdout human-friendly)")
	}
	if cfg.jsonlOut != "" && cfg.csvOut != "" && cfg.jsonlOut == cfg.csvOut {
		return config{}, fmt.Errorf("-jsonl-out and -csv-out must not be the same path")
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
	if banner := bannerFor(os.Stdout); banner != "" {
		b.WriteString(banner)
		b.WriteString("\n")
	}
	b.WriteString("Usage:\n  poke -url URL -prompts FILE [flags]\n\nFlags:\n")
	fs.SetOutput(&b)
	fs.PrintDefaults()
	return b.String()
}

func run(ctx context.Context, cfg config) error {
	tmpl, err := loadRequestTemplate(cfg)
	if err != nil {
		return err
	}
	cfg.reqTemplate = tmpl

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

	mcfg := defaultMarkerConfig()
	if cfg.markersFile != "" {
		loaded, err := loadMarkerConfigFile(cfg.markersFile)
		if err != nil {
			return err
		}
		mcfg = loaded
	}

	analyzer, err := newResponseAnalyzer(mcfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	sink, err := newResultSink(cfg.jsonlOut, cfg.csvOut)
	if err != nil {
		return err
	}
	defer func() {
		if sink != nil {
			_ = sink.Close()
		}
	}()

	stats := newReport(analyzer, mcfg.Categories, cancel, sink)

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
		readErr <- promptset.Stream(ctx, cfg.promptsFile, prompts, promptset.Options{})
	}()

	wg.Wait()

	if err := <-readErr; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	if sink != nil {
		if err := sink.Close(); err != nil {
			return err
		}
	}

	stats.LogSummary()
	if err := stats.ThresholdError(); err != nil {
		return err
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
	stats *report,
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

			res := sendOne(ctx, client, cfg, baseHeaders, cookies, workerID, prompt)
			stats.RecordResult(res)
		}
	}
}

func sendOne(
	ctx context.Context,
	client *http.Client,
	cfg config,
	baseHeaders http.Header,
	cookies []*http.Cookie,
	workerID int,
	prompt string,
) RequestResult {
	start := time.Now()

	u, bodyBytes, err := buildTargetURLAndBody(cfg, prompt)
	if err != nil {
		return RequestResult{WorkerID: workerID, Prompt: prompt, Latency: time.Since(start), Err: err}
	}

	var attempts int
	var retries int

	for {
		attempts++

		var body io.Reader
		if cfg.method != http.MethodGet && bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, cfg.method, u.String(), body)
		if err != nil {
			return RequestResult{WorkerID: workerID, Prompt: prompt, Attempts: attempts, Retries: retries, Latency: time.Since(start), Err: fmt.Errorf("build request: %w", err)}
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
			if cfg.retry.enabled() && retries < cfg.retry.MaxRetries && isRetryableDoError(err) {
				retries++
				delay := nextBackoffDelay(cfg.retry, retries, 0)
				if sleepErr := sleepCtx(ctx, delay); sleepErr != nil {
					return RequestResult{WorkerID: workerID, Prompt: prompt, Attempts: attempts, Retries: retries - 1, Latency: time.Since(start), Err: sleepErr}
				}
				continue
			}
			return RequestResult{WorkerID: workerID, Prompt: prompt, Attempts: attempts, Retries: retries, Latency: time.Since(start), Err: err}
		}

		if cfg.retry.enabled() && retries < cfg.retry.MaxRetries && isRetryableHTTPStatus(resp.StatusCode) {
			retryAfter, _ := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			_ = resp.Body.Close()

			retries++
			delay := nextBackoffDelay(cfg.retry, retries, retryAfter)
			if sleepErr := sleepCtx(ctx, delay); sleepErr != nil {
				return RequestResult{WorkerID: workerID, Prompt: prompt, Attempts: attempts, Retries: retries - 1, Latency: time.Since(start), Err: sleepErr}
			}
			continue
		}

		defer resp.Body.Close()

		b, truncated, err := readResponseBody(resp, cfg.maxRespBytes, cfg.streamResp)
		if err != nil {
			return RequestResult{WorkerID: workerID, Prompt: prompt, Attempts: attempts, Retries: retries, StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), Latency: time.Since(start), Err: fmt.Errorf("read response body: %w", err)}
		}
		return RequestResult{WorkerID: workerID, Prompt: prompt, Attempts: attempts, Retries: retries, StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), Latency: time.Since(start), Body: b, BodyTruncated: truncated}
	}
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
