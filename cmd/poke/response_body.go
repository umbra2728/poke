package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
)

func readResponseBody(resp *http.Response, maxBytes int64, stream bool) ([]byte, bool, error) {
	if resp == nil || resp.Body == nil {
		return nil, false, nil
	}
	if maxBytes < 0 {
		return nil, false, fmt.Errorf("maxBytes must be >= 0")
	}
	if maxBytes == 0 {
		b, err := io.ReadAll(resp.Body)
		return b, false, err
	}
	// bytes.Buffer and slice indexing use int; reject values that can't fit.
	maxInt := int64(^uint(0) >> 1)
	if maxBytes > maxInt {
		return nil, false, fmt.Errorf("maxBytes too large: %d (max %d)", maxBytes, maxInt)
	}
	if stream {
		return readResponseBodyStream(resp.Body, resp.ContentLength, maxBytes)
	}
	return readResponseBodyExact(resp.Body, maxBytes)
}

func readResponseBodyExact(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		b, err := io.ReadAll(r)
		return b, false, err
	}
	if maxBytes == math.MaxInt64 {
		// Can't add +1 safely; fall back to streaming semantics.
		return readResponseBodyStream(r, -1, maxBytes)
	}
	b, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > maxBytes {
		return b[:maxBytes], true, nil
	}
	return b, false, nil
}

func readResponseBodyStream(r io.Reader, contentLength int64, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		b, err := io.ReadAll(r)
		return b, false, err
	}

	var buf bytes.Buffer
	// Avoid eagerly allocating maxBytes (which might be large).
	const preallocCap = 64 * 1024
	if maxBytes < preallocCap {
		buf.Grow(int(maxBytes))
	} else {
		buf.Grow(preallocCap)
	}

	tmpFull := make([]byte, 32*1024)
	for buf.Len() < int(maxBytes) {
		remaining := int(maxBytes) - buf.Len()
		tmp := tmpFull
		if remaining < len(tmp) {
			tmp = tmp[:remaining]
		}
		n, err := r.Read(tmp)
		if n > 0 {
			_, _ = buf.Write(tmp[:n])
		}
		if err != nil {
			if err == io.EOF {
				return buf.Bytes(), false, nil
			}
			return nil, false, err
		}
	}

	// We stopped because we hit maxBytes. Avoid a "read one more byte" probe; it can
	// block on slow endpoints. Use Content-Length when available; otherwise, be
	// conservative and treat it as truncated.
	if contentLength >= 0 && contentLength <= maxBytes {
		return buf.Bytes(), false, nil
	}
	return buf.Bytes(), true, nil
}
