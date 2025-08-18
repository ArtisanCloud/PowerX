package manager

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (m *managerImpl) waitHealthy(ctx context.Context, baseURL, path, intervalStr, timeoutStr string) error {
	if strings.TrimSpace(path) == "" {
		return nil // 未声明健康检查则视为立即健康（MVP 简化）
	}
	interval := 2 * time.Second
	if d, err := time.ParseDuration(intervalStr); err == nil {
		interval = d
	}
	timeout := 10 * time.Second
	if d, err := time.ParseDuration(timeoutStr); err == nil {
		timeout = d
	}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(baseURL, "/") + path
	client := &http.Client{Timeout: interval}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("healthcheck timeout: %s", url)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
