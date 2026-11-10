// Package connectionprobe 仅使用只读接口验证供应商入口和鉴权，不依赖任何生成驱动。
package connectionprobe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	Models           = "models"
	KeyInfo          = "key_info"
	OllamaTags       = "ollama_tags"
	maxResponseBytes = 2 << 20
)

type Input struct {
	Provider, Endpoint, APIKey, Organization, Strategy string
}

type Result struct {
	Provider            string `json:"provider"`
	Probe               string `json:"probe"`
	EndpointReachable   bool   `json:"endpoint_reachable"`
	APIKeyVerified      bool   `json:"api_key_verified"`
	Authentication      string `json:"authentication"`
	ModelAccessVerified bool   `json:"model_access_verified"`
	GenerationSubmitted bool   `json:"generation_submitted"`
}

// Verify 在一个有限时长内最多执行两个 GET；不重试、不跟随重定向、不创建任务。
func Verify(ctx context.Context, in Input) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(in.Endpoint), "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, failure(422, "AI_CONNECTION_CONFIG_INVALID", 0)
	}
	var field string
	switch in.Strategy {
	case Models:
		u.Path += "/models"
		field = "data"
	case KeyInfo:
		u.Path += "/auth/key"
		field = "data"
	case OllamaTags:
		u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/v1"), "/api") + "/api/tags"
		field = "models"
	default:
		return nil, failure(501, "AI_CONNECTION_PROBE_UNSUPPORTED", 0)
	}
	if in.Strategy != OllamaTags && strings.TrimSpace(in.APIKey) == "" {
		return nil, failure(422, "AI_CONNECTION_CONFIG_INVALID", 0)
	}
	u.RawPath = ""
	// 地址已拒绝 userinfo/query/fragment，日志只记录有效目标，不记录凭据。
	logger.InfoF(ctx, "[ai.connection] probe target provider=%s source=%s", in.Provider, u.String())
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(key string) (int, []byte, error) {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if e != nil {
			return 0, nil, failure(422, "AI_CONNECTION_CONFIG_INVALID", 0)
		}
		req.Header.Set("Accept", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		if in.Organization != "" {
			req.Header.Set("OpenAI-Organization", in.Organization)
		}
		res, e := client.Do(req)
		if e != nil {
			if ctx.Err() != nil {
				return 0, nil, failure(504, "AI_CONNECTION_TIMEOUT", 0)
			}
			return 0, nil, failure(502, "AI_CONNECTION_UNREACHABLE", 0)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
		if e != nil && ctx.Err() != nil {
			return res.StatusCode, nil, failure(504, "AI_CONNECTION_TIMEOUT", res.StatusCode)
		}
		if e != nil || len(raw) > maxResponseBytes {
			return res.StatusCode, nil, failure(502, "AI_CONNECTION_RESPONSE_INVALID", res.StatusCode)
		}
		return res.StatusCode, raw, nil
	}
	status, raw, err := request(strings.TrimSpace(in.APIKey))
	if err != nil {
		return nil, err
	}
	switch {
	case status == 401 || status == 403:
		// 供应商 Key 失败不返回 Core 的 401，避免客户端刷新用户 JWT。
		return nil, failure(422, "AI_CONNECTION_AUTH_FAILED", status)
	case status == 404 || status == 405 || status == 501:
		return nil, failure(501, "AI_CONNECTION_PROBE_UNSUPPORTED", status)
	case status == 429:
		return nil, failure(429, "AI_CONNECTION_RATE_LIMITED", status)
	case status >= 300 && status < 400:
		return nil, failure(502, "AI_CONNECTION_REDIRECT_REJECTED", status)
	case status < 200 || status >= 300:
		return nil, failure(502, "AI_CONNECTION_UPSTREAM_FAILED", status)
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil {
		return nil, failure(502, "AI_CONNECTION_RESPONSE_INVALID", status)
	}
	value, exists := envelope[field]
	if !exists || string(value) == "null" {
		return nil, failure(502, "AI_CONNECTION_RESPONSE_INVALID", status)
	}
	if in.Strategy == KeyInfo {
		var object map[string]any
		if json.Unmarshal(value, &object) != nil || len(object) == 0 {
			return nil, failure(502, "AI_CONNECTION_RESPONSE_INVALID", status)
		}
	} else {
		var items []json.RawMessage
		if json.Unmarshal(value, &items) != nil || items == nil {
			return nil, failure(502, "AI_CONNECTION_RESPONSE_INVALID", status)
		}
	}
	result := &Result{Provider: in.Provider, Probe: in.Strategy, EndpointReachable: true, Authentication: "not_required"}
	if in.Strategy != OllamaTags {
		// 公开目录或接受任意 Bearer 的入口不能证明当前 Key 有效。
		controlStatus, _, controlErr := request("powerx-deliberately-invalid-probe-key")
		if controlErr != nil {
			return nil, controlErr
		}
		if controlStatus != 401 && controlStatus != 403 {
			return nil, failure(501, "AI_CONNECTION_AUTH_UNVERIFIABLE", controlStatus)
		}
		result.APIKeyVerified, result.Authentication = true, "api_key"
	}
	return result, nil
}

func failure(status int, code string, upstreamStatus int) *dto.AppError {
	details := map[string]any{"generation_submitted": false, "api_key_verified": false}
	if upstreamStatus > 0 {
		details["upstream_status"] = upstreamStatus
	}
	return &dto.AppError{HTTPCode: status, Code: code, Message: code, Details: details}
}
