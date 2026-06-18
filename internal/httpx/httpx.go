package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client returns c or http.DefaultClient when c is nil.
func Client(c *http.Client) *http.Client {
	if c == nil {
		return http.DefaultClient
	}
	return c
}

// JSONRequest builds an HTTP request with a JSON body.
func JSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
		r = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// RetryAfter parses the standard Retry-After header as seconds or HTTP date.
func RetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if now.IsZero() {
			now = time.Now()
		}
		if delay := when.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}
