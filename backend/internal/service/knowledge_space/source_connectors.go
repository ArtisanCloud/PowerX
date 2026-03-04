package knowledge_space

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SourceFetchRequest struct {
	Provider string
	BaseURL  string
	Token    string
	Scope    map[string]any
	Cursor   string
	Limit    int
}

type SourceFetchResponse struct {
	Units      []DocumentUnit
	NextCursor string
	HasMore    bool
}

type httpRetryClient struct {
	client       *http.Client
	minInterval  time.Duration
	maxAttempts  int
	backoffBase  time.Duration
	backoffMax   time.Duration
	lastRequest  time.Time
}

func newHTTPRetryClient(client *http.Client) *httpRetryClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &httpRetryClient{
		client:      client,
		minInterval: 200 * time.Millisecond,
		maxAttempts: 3,
		backoffBase: 200 * time.Millisecond,
		backoffMax:  2 * time.Second,
	}
}

func (c *httpRetryClient) do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	if req == nil {
		return nil, nil, errors.New("nil request")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req = req.WithContext(ctx)

	var payload []byte
	if req.Body != nil {
		payload, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}

	for attempt := 0; attempt < c.maxAttempts; attempt++ {
		// naive client-side throttle
		if !c.lastRequest.IsZero() {
			wait := c.minInterval - time.Since(c.lastRequest)
			if wait > 0 {
				time.Sleep(wait)
			}
		}
		c.lastRequest = time.Now()

		clone := req.Clone(ctx)
		if payload != nil {
			clone.Body = io.NopCloser(bytes.NewReader(payload))
		}
		resp, err := c.client.Do(clone)
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return resp, body, nil
		}
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			// Retry on 429/5xx.
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				time.Sleep(c.backoff(attempt, resp.Header.Get("Retry-After")))
				continue
			}
			return resp, body, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		time.Sleep(c.backoff(attempt, ""))
	}
	return nil, nil, errors.New("request failed after retries")
}

func (c *httpRetryClient) backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if n, err := time.ParseDuration(strings.TrimSpace(retryAfter) + "s"); err == nil && n > 0 {
			return minDuration(n, c.backoffMax)
		}
	}
	d := c.backoffBase * time.Duration(1<<attempt)
	return minDuration(d, c.backoffMax)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func jsonMap(body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

