package promptset

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxPromptBytes = 1 << 20 // 1 MiB

type Options struct {
	Mutate      bool
	MaxVariants int // max variants per seed (including the original); <= 0 means "no limit"
}

func Stream(ctx context.Context, path string, out chan<- string, opt Options) error {
	r, closeFn, err := openPath(path)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return streamJSON(ctx, r, out, opt)
	case ".jsonl", ".ndjson":
		return streamJSONL(ctx, r, out, opt)
	default:
		return streamText(ctx, r, out, opt)
	}
}

func streamText(ctx context.Context, r io.Reader, out chan<- string, opt Options) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxPromptBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := emitPrompt(ctx, out, line, opt); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read prompts: %w", err)
	}
	return nil
}

type jsonPromptItem struct {
	Prompt   string   `json:"prompt"`
	Disabled bool     `json:"disabled,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	ID       string   `json:"id,omitempty"`
}

func streamJSON(ctx context.Context, r io.Reader, out chan<- string, opt Options) error {
	var root any
	dec := json.NewDecoder(r)
	if err := dec.Decode(&root); err != nil {
		return fmt.Errorf("read prompts json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("read prompts json: extra trailing content")
		}
		return fmt.Errorf("read prompts json: %w", err)
	}

	items, err := parsePromptJSON(root)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Disabled {
			continue
		}
		if strings.TrimSpace(it.Prompt) == "" {
			return fmt.Errorf("read prompts json: empty prompt")
		}
		if err := emitPrompt(ctx, out, it.Prompt, opt); err != nil {
			return err
		}
	}
	return nil
}

func parsePromptJSON(root any) ([]jsonPromptItem, error) {
	switch x := root.(type) {
	case []any:
		return parsePromptJSONArray(x)
	case map[string]any:
		raw, ok := x["prompts"]
		if !ok {
			return nil, fmt.Errorf("read prompts json: expected top-level array, or object with \"prompts\"")
		}
		arr, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("read prompts json: \"prompts\" must be an array")
		}
		return parsePromptJSONArray(arr)
	default:
		return nil, fmt.Errorf("read prompts json: expected top-level array, or object with \"prompts\"")
	}
}

func parsePromptJSONArray(arr []any) ([]jsonPromptItem, error) {
	out := make([]jsonPromptItem, 0, len(arr))
	for i, v := range arr {
		switch vv := v.(type) {
		case string:
			out = append(out, jsonPromptItem{Prompt: vv})
		case map[string]any:
			p, ok := vv["prompt"]
			if !ok {
				return nil, fmt.Errorf("read prompts json: item[%d]: missing \"prompt\"", i)
			}
			ps, ok := p.(string)
			if !ok {
				return nil, fmt.Errorf("read prompts json: item[%d]: \"prompt\" must be a string", i)
			}
			disabled, _ := vv["disabled"].(bool)
			out = append(out, jsonPromptItem{Prompt: ps, Disabled: disabled})
		default:
			return nil, fmt.Errorf("read prompts json: item[%d]: expected string or object", i)
		}
	}
	return out, nil
}

func streamJSONL(ctx context.Context, r io.Reader, out chan<- string, opt Options) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	// JSONL lines can be larger than plain prompts (metadata, escaping).
	sc.Buffer(buf, 2*maxPromptBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var prompt string
		switch line[0] {
		case '"':
			if err := json.Unmarshal([]byte(line), &prompt); err != nil {
				return fmt.Errorf("read prompts jsonl: invalid json string: %w", err)
			}
		case '{':
			var it jsonPromptItem
			if err := json.Unmarshal([]byte(line), &it); err != nil {
				return fmt.Errorf("read prompts jsonl: invalid json object: %w", err)
			}
			if it.Disabled {
				continue
			}
			prompt = it.Prompt
		default:
			return fmt.Errorf("read prompts jsonl: each non-empty line must be a JSON string or object")
		}

		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("read prompts jsonl: empty prompt")
		}
		if err := emitPrompt(ctx, out, prompt, opt); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read prompts jsonl: %w", err)
	}
	return nil
}

func emitPrompt(ctx context.Context, out chan<- string, prompt string, opt Options) error {
	if !opt.Mutate {
		return send(ctx, out, prompt)
	}
	variants := Mutate(prompt, opt.MaxVariants)
	for _, v := range variants {
		if err := send(ctx, out, v); err != nil {
			return err
		}
	}
	return nil
}

func send(ctx context.Context, out chan<- string, prompt string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- prompt:
		return nil
	}
}

func openPath(path string) (r io.Reader, closeFn func() error, err error) {
	if path == "-" {
		return os.Stdin, nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open prompts file: %w", err)
	}
	return f, f.Close, nil
}
