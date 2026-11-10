package agnes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// DefaultEndpoint 是 Agnes 国际站媒体 API 的正式入口。
const DefaultEndpoint = "https://apihub.agnes-ai.com/v1"

func modelConfig(runtime map[string]any) (*config.ModelConfig, error) {
	for _, key := range []string{"config", "model_config"} {
		switch v := runtime[key].(type) {
		case *config.ModelConfig:
			if v != nil {
				return v, nil
			}
		case config.ModelConfig:
			return &v, nil
		}
	}
	return nil, fmt.Errorf("agnes runtime config missing")
}

// apiURL 仅从 Core 保存的连接配置构建路径，不接受供应商响应提供的任务地址。
func apiURL(mc *config.ModelConfig, path string) (*url.URL, error) {
	base := strings.TrimRight(strings.TrimSpace(mc.Endpoint), "/")
	if base == "" {
		base = DefaultEndpoint
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("agnes endpoint invalid")
	}
	if strings.TrimSpace(mc.APIKey) == "" {
		return nil, fmt.Errorf("agnes credential missing")
	}
	prefix := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
	u.Path = prefix + path
	u.RawPath = ""
	return u, nil
}

// call 不重试提交，也不携带凭据跟随重定向。原始响应及签名 URL 不进入错误文案。
func call(ctx context.Context, mc *config.ModelConfig, method string, endpoint *url.URL, payload, out any, limit int64) (http.Header, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("agnes request invalid")
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(mc.APIKey))
	req.Header.Set("Content-Type", "application/json")
	timeout := mc.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("agnes provider transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return nil, core.MediaHTTPError(response.StatusCode, response.Header, raw)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, outputError()
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return nil, outputError()
	}
	return response.Header, nil
}

func outputError() *dto.AppError {
	return &dto.AppError{HTTPCode: 502, Code: "AI_OUTPUT_INVALID", Message: "AI_OUTPUT_INVALID"}
}
func parameterError(field, reason string) *dto.AppError {
	return &dto.AppError{HTTPCode: 422, Code: "AI_PARAMETER_INVALID", Message: "AI_PARAMETER_INVALID", Details: map[string]any{"parameter": field, "reason": reason}}
}
func setting(runtime map[string]any, mc *config.ModelConfig, key, fallback string) string {
	if value, ok := runtime[key].(string); ok && value != "" {
		return value
	}
	if value, ok := mc.Extra[key].(string); ok && value != "" {
		return value
	}
	return fallback
}
func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
