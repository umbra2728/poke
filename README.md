# poke

Black-box prompt fuzzer for user-facing LLM-ish HTTP endpoints. Targets any URL you pass (GET or POST by default), sprays prompts, and spots risky responses via heuristics.

## Quickstart

- Build: `go build -o poke ./cmd/poke`
- Run with markers + structured outputs:
  - `./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.jsonl -markers-file markers.example.json -jsonl-out poke.results.jsonl -csv-out poke.results.csv -ci-exit-codes`

## Flags

- `-url` (required): target endpoint.
- `-method`: HTTP method (default POST).
- `-prompts` (required): prompt file or `-` for stdin.
- `-headers-file`: `Header-Name: value` per line.
- `-cookies-file`: `name=value` per line.
- `-markers-file`: markers config JSON (regexes + per-category thresholds); see `markers.example.json`.
- `-body-template`: JSON request body template (non-GET); supports `{{prompt}}`.
- `-body-template-file`: file path to JSON request body template (non-GET); supports `{{prompt}}`.
- `-query-template`: URL query template (`k=v&k2=v2`); values support `{{prompt}}`.
- `-query-template-file`: file path to URL query template; values support `{{prompt}}`.
- `-jsonl-out`: write per-request results as JSONL to a file.
- `-csv-out`: write per-request results as CSV to a file.
- `-ci-exit-codes`: map marker stop thresholds to CI-friendly exit codes (2=warn/info, 3=error, 4=critical).
- `-workers`: concurrent workers (default 10).
- `-rate`: global RPS cap, 0 = unlimited.
- `-timeout`: per-request timeout.
- `-retries`: max retries for transport errors/429/5xx; `0` = disabled.
- `-backoff-min`: minimum retry backoff delay.
- `-backoff-max`: maximum retry backoff delay; `0` = no cap.
- `-max-response-bytes`: max response bytes to read/store/analyze; `0` = unlimited.
- `-stream-response`: stream response body reads and truncate at `-max-response-bytes` (faster; truncation may be conservative).

## Request shape

- Default behavior (no templates):
  - `GET`: attaches `?prompt=...`.
  - Non-`GET`: sends JSON `{"prompt": "..."}` and sets `Content-Type: application/json` unless you override via headers.
- With templates:
  - `-body-template` / `-body-template-file`: provide a valid JSON value (object/array/etc). Any string value may include `{{prompt}}`, which is replaced before JSON marshaling (so the prompt is escaped safely). Not supported with `-method GET`.
  - `-query-template` / `-query-template-file`: provide `k=v&k2=v2` (optional leading `?`). Values may include `{{prompt}}` and will be URL-encoded via `url.Values`.

Example body template:
`{"model":"my-model","messages":[{"role":"user","content":"{{prompt}}"}]}`

Example query template:
`model=my-model&prompt={{prompt}}`

## Common recipes

- Retry on flaky endpoints / 429 / 5xx (exponential backoff + jitter; honors `Retry-After`): `-retries 2 -backoff-min 200ms -backoff-max 5s`
- POST with a JSON body template:
  - `./poke -url https://example.com/chat -prompts corpus/seed_prompts.jsonl -body-template-file examples/body-template.example.json`
- GET with a query template:
  - `./poke -url https://example.com/search -method GET -prompts corpus/seed_prompts.jsonl -query-template 'q={{prompt}}&mode=debug'`

## Inputs

- Prompts file:
  - `.txt`: one prompt per line; blank lines and `#` comments are ignored.
  - `.json`: either a top-level array of prompts, or an object with `"prompts": [...]`. Items may be strings or objects like `{"prompt":"...","disabled":false}`.
  - `.jsonl` / `.ndjson`: one JSON value per line; each line is either a JSON string or an object like `{"prompt":"...","disabled":false}`. Blank lines and `#` comments are ignored.
- Headers file: `Key: Value` lines, canonicalized.
- Cookies file: `name=value` lines.

## Output & detection

- Progress log every 100 requests.
- Final summary: HTTP status counts, latency min/avg/max, overall severity, marker counts, top offending responses (prompt + response preview).
- Optional per-request structured output via `-jsonl-out` / `-csv-out` (written to files; stdout stays human-friendly).
- Marker categories include jailbreak success, system/internal leak hints, PII patterns, credential/key material, file path/env hints, HTTP 4xx/5xx, and rate-limit signals (429/Retry-After/phrases).

### Structured output schemas

- JSONL: one JSON object per request (keys: `time`, `seq`, `worker_id`, `prompt`, `attempts`, `retries`, `status_code`, `latency_ms`, `body_len`, `body_truncated`, `body_preview`, `error`, `marker_hits`, `score`, `severity`).
  - `marker_hits` is an array of objects with keys `ID`, `Category`, `Count`.
- CSV: stable columns: `time,seq,worker_id,attempts,retries,status_code,latency_ms,body_len,body_truncated,severity,score,marker_hits,error,prompt,body_preview`
  - `marker_hits` is a `;`-separated `id=count` list (e.g. `jwt=1;email_address=2`).
- Note: `-jsonl-out` / `-csv-out` only support file paths; `-` is not supported (stdout stays human-friendly).

## Markers & thresholds

Markers are regex-driven and configurable at runtime via `-markers-file` (JSON).

- By default, `-markers-file` *merges* with the built-in marker set (override by matching `category` + `id`, or add new ones).
- Set `"replace_defaults": true` to provide a fully custom regex set.
- Per-category thresholds can stop the run early (`stop_after_responses` / `stop_after_matches`) or elevate the run's reported severity (`elevate_after_responses` + `elevate_to`).
- With `-ci-exit-codes`, runs that stop due to a category threshold exit with 2/3/4 based on that category's configured severity.

## Exit codes

- Default: `0` on completion, `1` on errors (including threshold stops).
- With `-ci-exit-codes`: threshold stops exit `2`/`3`/`4` for warn-or-info / error / critical categories (other failures still exit `1`).

## CI (GitHub Actions)

See `examples/github-actions-poke.yml` for a stub workflow that runs `poke`, uploads `-jsonl-out`/`-csv-out`, and gates on exit codes.

## Roadmap ideas

- Configurable body schema (templated JSON/query parameters beyond `prompt`). (implemented)
- Jittered rate limiting for sturdier runs.
- Richer markers (file/path/key leaks, PII snippets).
- Auth helpers: header/cookie presets and env var expansion.
