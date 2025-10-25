package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Config 定义企业 IM Webhook 发送配置。
type Config struct {
	WebhookURL    string
	RetryInterval time.Duration
	MaxRetry      int
	HTTPTimeout   time.Duration
}

// Message 表示要发送的通知内容。
type Message struct {
	Title    string         `json:"title"`
	Content  string         `json:"content"`
	Severity string         `json:"severity,omitempty"`
	TraceID  string         `json:"trace_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Sender 负责向企业 IM 渠道发送通知。
type Sender struct {
	cfg        Config
	httpClient *http.Client
}

// NewSender 创建 Sender。
func NewSender(cfg Config) *Sender {
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 30 * time.Second
	}
	if cfg.MaxRetry < 0 {
		cfg.MaxRetry = 0
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 5 * time.Second
	}
	return &Sender{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
	}
}

// Send 将消息推送到 Webhook。
func (s *Sender) Send(ctx context.Context, msg Message) error {
	if s.cfg.WebhookURL == "" {
		return errors.New("im webhook url not configured")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal im payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	attempts := s.cfg.MaxRetry + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, doErr := s.httpClient.Do(req)
		if doErr != nil {
			if attempt == attempts {
				return fmt.Errorf("send webhook request: %w", doErr)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.cfg.RetryInterval):
				continue
			}
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if attempt == attempts {
			return fmt.Errorf("im webhook responded with status %d", resp.StatusCode)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.cfg.RetryInterval):
			continue
		}
	}
	return nil
}
