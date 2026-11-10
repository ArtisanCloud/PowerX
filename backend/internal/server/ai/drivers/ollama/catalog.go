package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const catalogResponseLimit = 2 << 20

var errCatalogModelUnavailable = errors.New("ollama model unavailable")

// ListModels 只查询目标实例的 tags/show，不加载模型、不提交推理或下载请求。
// 能力由实例明确声明，不能根据型号名称猜测向量或视觉支持。
func ListModels(ctx context.Context, endpoint, modality string) ([]string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, catalogError(422, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
	}
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/v1"), "/api")
	u.RawPath = ""
	base := strings.TrimRight(u.String(), "/")
	capability := ""
	switch modality {
	case "llm":
		capability = "completion"
	case "embedding":
		capability = "embedding"
	case "image", "vlm":
		capability = "vision"
	default:
		return []string{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	read := func(method, path string, body []byte, out any) error {
		req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
		if err != nil {
			return catalogError(422, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return catalogError(504, "AI_MODEL_DISCOVERY_TIMEOUT")
			}
			return catalogError(502, "AI_MODEL_DISCOVERY_UNREACHABLE")
		}
		defer resp.Body.Close()
		// tags 与 show 之间型号可能被删除，云型号也可能已下线。
		if path == "/api/show" && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone) {
			return errCatalogModelUnavailable
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return catalogError(502, "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, catalogResponseLimit+1))
		if ctx.Err() != nil {
			return catalogError(504, "AI_MODEL_DISCOVERY_TIMEOUT")
		}
		if err != nil || len(raw) > catalogResponseLimit || json.Unmarshal(raw, out) != nil {
			return catalogError(502, "AI_MODEL_DISCOVERY_RESPONSE_INVALID")
		}
		return nil
	}
	var tags struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := read(http.MethodGet, "/api/tags", nil, &tags); err != nil {
		return nil, err
	}
	if tags.Models == nil {
		return nil, catalogError(502, "AI_MODEL_DISCOVERY_RESPONSE_INVALID")
	}
	models := []string{}
	seen := map[string]bool{}
	for _, item := range tags.Models {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.Model)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		body, _ := json.Marshal(map[string]string{"model": name})
		var details struct {
			Capabilities []string `json:"capabilities"`
		}
		if err := read(http.MethodPost, "/api/show", body, &details); err != nil {
			if errors.Is(err, errCatalogModelUnavailable) {
				continue
			}
			return nil, err
		}
		if len(details.Capabilities) == 0 {
			return nil, catalogError(502, "AI_MODEL_DISCOVERY_CAPABILITIES_UNAVAILABLE")
		}
		for _, supported := range details.Capabilities {
			if supported == capability {
				models = append(models, name)
				break
			}
		}
	}
	return models, nil
}

func catalogError(status int, code string) error {
	return &dto.AppError{HTTPCode: status, Code: code, Message: code}
}
