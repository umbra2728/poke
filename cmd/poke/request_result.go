package main

import (
	"net/http"
	"time"
)

type RequestResult struct {
	WorkerID   int
	Prompt     string
	StatusCode int
	Headers    http.Header
	Latency    time.Duration
	Body       []byte
	Err        error
}
