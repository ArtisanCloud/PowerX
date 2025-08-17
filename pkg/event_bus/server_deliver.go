package event_bus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *httpServer) deliver(ctx context.Context, sub Subscriber, env Envelope) {
	body, _ := json.Marshal(env)
	backoff := s.baseBackoff
	var lastErr error
	var status int

	for i := 0; i < s.maxAttempts; i++ {
		reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
		req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.URL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// TODO: 在此加入 HMAC/JWT 签名头，插件 /events 校验来源

		resp, err := s.httpClient.Do(req)
		cancel()
		if err == nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
			if status >= 200 && status < 300 {
				s.auditor.LogDeliver(ctx, env.Topic, sub.PluginID, status, "")
				return
			}
			if status >= 400 && status < 500 {
				lastErr = fmt.Errorf("non-retryable %d", status)
				break
			}
			lastErr = fmt.Errorf("retryable %d", status)
		} else {
			lastErr = err
		}

		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			s.auditor.LogDeliver(ctx, env.Topic, sub.PluginID, status, "context canceled")
			return
		}
	}
	msg := ""
	if lastErr != nil {
		msg = lastErr.Error()
	}
	s.auditor.LogDeliver(ctx, env.Topic, sub.PluginID, status, msg)
}
