Black-box prompt fuzzer for user-facing LLM-ish HTTP endpoints. Targets any URL you pass (GET or POST by default), sprays prompts, and spots risky responses via heuristics.

## Quickstart
- Build: `go build ./...`
- Basic run (POST JSON): `./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.txt -workers 20 -rate 10 -timeout 15s`
- With prompt mutations: `./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.txt -mutate -mutate-max 12 -workers 20 -rate 10 -timeout 15s`

## Flags
- `-url` (required): target endpoint.
- `-method`: HTTP method (default POST).
- `-prompts` (required): prompt file or `-` for stdin.
- `-headers-file`: `Header-Name: value` per line.
- `-cookies-file`: `name=value` per line.
- `-workers`: concurrent workers (default 10).
- `-rate`: global RPS cap, 0 = unlimited.
- `-timeout`: per-request timeout.
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
- Final summary: HTTP status counts, latency min/avg/max, marker counts, top offending responses (prompt + response preview).
- Marker categories: jailbreak success (ignore/bypass/system prompt reveals), system/internal leak hints, HTTP 4xx/5xx, and rate-limit signals (429/Retry-After/phrases).

## Prompt mutations
Lightweight generators add noisy prefixes/suffixes, delimiter tweaks, and role swaps to widen coverage without hand-writing every payload.

## Roadmap ideas
- Configurable body schema (templated JSON/query parameters beyond `prompt`).
- Retry/backoff and jittered rate limiting for sturdier runs.
- Richer markers (file/path/key leaks, PII snippets) and per-category thresholds for exits.
- Optional structured output (JSONL) for pipeline ingestion and CI gating.
- Auth helpers: header/cookie presets and env var expansion.
