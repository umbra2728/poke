Bootstrap Go CLI fuzzer skeleton.

Build:
  go build ./...

Run (POST JSON by default):
  ./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.txt -workers 20 -rate 10 -timeout 15s

Run with simple prompt mutations:
  ./poke -url http://localhost:8080/llm -prompts corpus/seed_prompts.txt -mutate -mutate-max 12 -workers 20 -rate 10 -timeout 15s

Files:
- `-prompts`: one prompt per line (blank lines / `#` comments ignored)
- `-headers-file`: `Header-Name: value` per line
- `-cookies-file`: `name=value` per line
- `-mutate`: generate simple variants (prefix/suffix noise, role swaps, delimiter changes)
- `-mutate-max`: cap variants per seed prompt (including original)

Behavior:
- `GET`: sends `?prompt=...`
- non-`GET`: sends JSON body `{"prompt":"..."}` and sets `Content-Type: application/json` unless overridden
