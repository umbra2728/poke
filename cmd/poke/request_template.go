package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const promptPlaceholder = "{{prompt}}"

type requestTemplate struct {
	body  *jsonBodyTemplate
	query *queryTemplate
}

func loadRequestTemplate(cfg config) (requestTemplate, error) {
	if cfg.bodyTmplStr == "" && cfg.bodyTmplFile == "" && cfg.queryTmplStr == "" && cfg.queryTmplFile == "" {
		return requestTemplate{}, nil
	}

	if cfg.method == http.MethodGet && (cfg.bodyTmplStr != "" || cfg.bodyTmplFile != "") {
		return requestTemplate{}, fmt.Errorf("-body-template is not supported with -method GET (GET requests do not send a body)")
	}

	var out requestTemplate

	if cfg.bodyTmplStr != "" || cfg.bodyTmplFile != "" {
		s, err := loadTemplateText(cfg.bodyTmplStr, cfg.bodyTmplFile, "body template")
		if err != nil {
			return requestTemplate{}, err
		}
		t, err := parseJSONBodyTemplate(s)
		if err != nil {
			return requestTemplate{}, err
		}
		out.body = t
	}

	if cfg.queryTmplStr != "" || cfg.queryTmplFile != "" {
		s, err := loadTemplateText(cfg.queryTmplStr, cfg.queryTmplFile, "query template")
		if err != nil {
			return requestTemplate{}, err
		}
		t, err := parseQueryTemplate(s)
		if err != nil {
			return requestTemplate{}, err
		}
		out.query = t
	}

	return out, nil
}

func loadTemplateText(inline string, path string, label string) (string, error) {
	if inline != "" && path != "" {
		return "", fmt.Errorf("%s: specify either inline or file", label)
	}
	if inline != "" {
		if strings.TrimSpace(inline) == "" {
			return "", fmt.Errorf("%s: template is empty", label)
		}
		return inline, nil
	}
	if path == "" {
		return "", fmt.Errorf("%s: missing template (internal error)", label)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: read %q: %w", label, path, err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", fmt.Errorf("%s: %q is empty", label, path)
	}
	return s, nil
}

// buildTargetURLAndBody applies default behavior or user-provided request templates.
//
// Defaults (backward compatible):
// - GET: attaches ?prompt=...
// - non-GET: sends JSON {"prompt": "..."} with Content-Type: application/json (unless overridden via headers).
func buildTargetURLAndBody(cfg config, prompt string) (*url.URL, []byte, error) {
	u, err := url.Parse(cfg.targetURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse -url: %w", err)
	}

	if cfg.reqTemplate.query != nil {
		if err := cfg.reqTemplate.query.Apply(u, prompt); err != nil {
			return nil, nil, err
		}
	} else if cfg.method == http.MethodGet {
		q := u.Query()
		q.Set(defaultJSONKey, prompt)
		u.RawQuery = q.Encode()
	}

	if cfg.method == http.MethodGet {
		return u, nil, nil
	}

	if cfg.reqTemplate.body != nil {
		b, err := cfg.reqTemplate.body.Render(prompt)
		if err != nil {
			return nil, nil, err
		}
		return u, b, nil
	}

	payload := map[string]string{defaultJSONKey: prompt}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal default json payload: %w", err)
	}
	return u, b, nil
}

type jsonBodyTemplate struct {
	root any
}

func parseJSONBodyTemplate(s string) (*jsonBodyTemplate, error) {
	var root any
	dec := json.NewDecoder(strings.NewReader(s))
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("body template: invalid JSON: %w", err)
	}
	// Ensure there is exactly one JSON value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("body template: invalid JSON: extra trailing content")
		}
		return nil, fmt.Errorf("body template: invalid JSON: %w", err)
	}
	return &jsonBodyTemplate{root: root}, nil
}

func (t *jsonBodyTemplate) Render(prompt string) ([]byte, error) {
	out := replacePlaceholdersInJSON(t.root, prompt)
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("body template: render: %w", err)
	}
	return b, nil
}

func replacePlaceholdersInJSON(v any, prompt string) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, vv := range x {
			m[k] = replacePlaceholdersInJSON(vv, prompt)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = replacePlaceholdersInJSON(x[i], prompt)
		}
		return out
	case string:
		if strings.Contains(x, promptPlaceholder) {
			return strings.ReplaceAll(x, promptPlaceholder, prompt)
		}
		return x
	default:
		return v
	}
}

type queryTemplate struct {
	values url.Values
}

func parseQueryTemplate(s string) (*queryTemplate, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "?") {
		s = strings.TrimPrefix(s, "?")
	}
	vs, err := url.ParseQuery(s)
	if err != nil {
		return nil, fmt.Errorf("query template: invalid query string: %w", err)
	}
	return &queryTemplate{values: vs}, nil
}

func (t *queryTemplate) Apply(u *url.URL, prompt string) error {
	if u == nil {
		return fmt.Errorf("query template: nil url (internal error)")
	}
	q := u.Query()
	for k, vals := range t.values {
		q.Del(k)
		for _, raw := range vals {
			v := raw
			if strings.Contains(v, promptPlaceholder) {
				v = strings.ReplaceAll(v, promptPlaceholder, prompt)
			}
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return nil
}
