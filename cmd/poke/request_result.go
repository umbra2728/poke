package main

import (
	"net/http"
	"time"
)

type RequestResult struct {
	WorkerID      int
	Prompt        string
	Attempts      int
	Retries       int
	StatusCode    int
	Headers       http.Header
	Latency       time.Duration
	Body          []byte
	BodyTruncated bool
	Err           error
}
