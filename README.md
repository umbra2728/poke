Bootstrap Go CLI fuzzer skeleton.

Build:
  go build ./...

Run (POST JSON by default):
  ./poke -url http://localhost:8080/llm -prompts prompts.txt -workers 20 -rate 10 -timeout 15s

Files:
- `-prompts`: one prompt per line (blank lines / `#` comments ignored)
- `-headers-file`: `Header-Name: value` per line
- `-cookies-file`: `name=value` per line

Behavior:
- `GET`: sends `?prompt=...`
- non-`GET`: sends JSON body `{"prompt":"..."}` and sets `Content-Type: application/json` unless overridden
