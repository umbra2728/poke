package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

func TestBuildTargetURLAndBody_DefaultGET(t *testing.T) {
	cfg := config{
		targetURL: "https://example.test/api",
		method:    http.MethodGet,
	}
	tmpl, err := loadRequestTemplate(cfg)
	if err != nil {
		t.Fatalf("loadRequestTemplate: %v", err)
	}
	cfg.reqTemplate = tmpl

	prompt := "hello world"
	u, body, err := buildTargetURLAndBody(cfg, prompt)
	if err != nil {
		t.Fatalf("buildTargetURLAndBody: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body, got %q", string(body))
	}
	got, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if got.Get(defaultJSONKey) != prompt {
		t.Fatalf("expected %q=%q, got %q", defaultJSONKey, prompt, got.Get(defaultJSONKey))
	}
}

func TestBuildTargetURLAndBody_DefaultPOST(t *testing.T) {
	cfg := config{
		targetURL: "https://example.test/api",
		method:    http.MethodPost,
	}
	tmpl, err := loadRequestTemplate(cfg)
	if err != nil {
		t.Fatalf("loadRequestTemplate: %v", err)
	}
	cfg.reqTemplate = tmpl

	prompt := "hello \"x\"\nline2"
	_, body, err := buildTargetURLAndBody(cfg, prompt)
	if err != nil {
		t.Fatalf("buildTargetURLAndBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("Unmarshal: %v (body=%q)", err, string(body))
	}
	if got[defaultJSONKey] != prompt {
		t.Fatalf("expected %q=%q, got %#v", defaultJSONKey, prompt, got[defaultJSONKey])
	}
}

func TestBuildTargetURLAndBody_BodyTemplate(t *testing.T) {
	cfg := config{
		targetURL:   "https://example.test/api",
		method:      http.MethodPost,
		bodyTmplStr: `{"messages":[{"role":"user","content":"prefix {{prompt}} suffix"}]}`,
	}
	tmpl, err := loadRequestTemplate(cfg)
	if err != nil {
		t.Fatalf("loadRequestTemplate: %v", err)
	}
	cfg.reqTemplate = tmpl

	prompt := "A&B \"C\"\n"
	_, body, err := buildTargetURLAndBody(cfg, prompt)
	if err != nil {
		t.Fatalf("buildTargetURLAndBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("Unmarshal: %v (body=%q)", err, string(body))
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected messages[0], got %#v", got["messages"])
	}
	msg0, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected messages[0] object, got %#v", msgs[0])
	}
	want := "prefix " + prompt + " suffix"
	if msg0["content"] != want {
		t.Fatalf("expected content=%q, got %#v", want, msg0["content"])
	}
}

func TestBuildTargetURLAndBody_QueryTemplate(t *testing.T) {
	cfg := config{
		targetURL:    "https://example.test/api?existing=1",
		method:       http.MethodGet,
		queryTmplStr: `model=my-model&prompt={{prompt}}`,
	}
	tmpl, err := loadRequestTemplate(cfg)
	if err != nil {
		t.Fatalf("loadRequestTemplate: %v", err)
	}
	cfg.reqTemplate = tmpl

	prompt := "A&B \"C\" \n"
	u, body, err := buildTargetURLAndBody(cfg, prompt)
	if err != nil {
		t.Fatalf("buildTargetURLAndBody: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body for GET, got %q", string(body))
	}
	got, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	want := url.Values{
		"existing": []string{"1"},
		"model":    []string{"my-model"},
		"prompt":   []string{prompt},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected query: got %#v want %#v (raw=%q)", got, want, u.RawQuery)
	}
}

func TestLoadRequestTemplate_BodyTemplateGETRejected(t *testing.T) {
	cfg := config{
		targetURL:   "https://example.test/api",
		method:      http.MethodGet,
		bodyTmplStr: `{"prompt":"{{prompt}}"}`,
	}
	if _, err := loadRequestTemplate(cfg); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadRequestTemplate_InvalidBodyTemplateRejected(t *testing.T) {
	cfg := config{
		targetURL:   "https://example.test/api",
		method:      http.MethodPost,
		bodyTmplStr: `{"prompt":}`,
	}
	if _, err := loadRequestTemplate(cfg); err == nil {
		t.Fatalf("expected error")
	}
}
