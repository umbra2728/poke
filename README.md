Black-box prompt fuzzer for user-facing LLM-ish HTTP endpoints. Targets any URL you pass (GET or POST by default), sprays prompts, and spots risky responses via heuristics.

## Quickstart
- Build: `go build ./cmd/poke`
- Basic run (POST JSON): `./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.txt -workers 20 -rate 10 -timeout 15s`
- With prompt mutations: `./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.txt -mutate -mutate-max 12 -workers 20 -rate 10 -timeout 15s`

## Flags
- `-url` (required): target endpoint.
- `-method`: HTTP method (default POST).
- `-prompts` (required): prompt file or `-` for stdin.
- `-headers-file`: `Header-Name: value` per line.
- `-cookies-file`: `name=value` per line.
- `-markers-file`: markers config JSON (regexes + per-category thresholds); see `markers.example.json`.
- `-jsonl-out`: write per-request results as JSONL to a file.
- `-csv-out`: write per-request results as CSV to a file.
- `-ci-exit-codes`: map marker stop thresholds to CI-friendly exit codes (2=warn/info, 3=error, 4=critical).
- `-workers`: concurrent workers (default 10).
- `-rate`: global RPS cap, 0 = unlimited.
- `-timeout`: per-request timeout.
- `-retries`: max retries for transport errors/429/5xx; `0` = disabled.
- `-backoff-min`: minimum retry backoff delay.
- `-backoff-max`: maximum retry backoff delay; `0` = no cap.
- `-mutate`: enable simple mutations (prefix/suffix noise, role swaps, delimiter changes).
- `-mutate-max`: cap variants per seed prompt (including the original); `<=0` means unlimited.

## Request shape
- `GET`: attaches `?prompt=...`.
- Non-`GET`: sends JSON `{"prompt": "..."}` and sets `Content-Type: application/json` unless you override via headers.

## Inputs
- Prompts file: one prompt per line; blank lines and `#` comments are ignored.
- Headers file: `Key: Value` lines, canonicalized.
- Cookies file: `name=value` lines.

## Output & detection
- Progress log every 100 requests.
- Final summary: HTTP status counts, latency min/avg/max, overall severity, marker counts, top offending responses (prompt + response preview).
- Optional per-request structured output via `-jsonl-out` / `-csv-out` (written to files; stdout stays human-friendly).
- Marker categories include jailbreak success, system/internal leak hints, PII patterns, credential/key material, file path/env hints, HTTP 4xx/5xx, and rate-limit signals (429/Retry-After/phrases).

## Markers & thresholds
Markers are regex-driven and configurable at runtime via `-markers-file` (JSON).

- By default, `-markers-file` *merges* with the built-in marker set (override by matching `category` + `id`, or add new ones).
- Set `"replace_defaults": true` to provide a fully custom regex set.
- Per-category thresholds can stop the run early (`stop_after_responses` / `stop_after_matches`) or elevate the run's reported severity (`elevate_after_responses` + `elevate_to`).
- With `-ci-exit-codes`, runs that stop due to a category threshold exit with 2/3/4 based on that category's configured severity.

## CI (GitHub Actions)
See `examples/github-actions-poke.yml` for a stub workflow that runs `poke`, uploads `-jsonl-out`/`-csv-out`, and gates on exit codes.

## Prompt mutations
Lightweight generators add noisy prefixes/suffixes, delimiter tweaks, and role swaps to widen coverage without hand-writing every payload.

## Roadmap ideas
- Configurable body schema (templated JSON/query parameters beyond `prompt`).
- Jittered rate limiting for sturdier runs.
- Richer markers (file/path/key leaks, PII snippets).
- Auth helpers: header/cookie presets and env var expansion.
