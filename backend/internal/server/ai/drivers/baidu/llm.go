package baidu

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"io"
	"net/http"
	"strings"
	"time"
)

// 百度（文心）Chat Completions：
// - Invoke：POST  /rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions?access_token=XXX
//           body: {messages:[{role,content}...], stream:false, temperature?, max_output_tokens?}
//           resp: {result: "..."} 或 choices[0].message.content
// - Stream：同上但 stream:true，服务端以逐行 JSON（或 "data: {...}"）返回增量；
//           每行里可能有 result（增量片段），或 is_end=true / done / [DONE] 结束。

type baiduClient struct{}

func NewLLMClient() *baiduClient { return &baiduClient{} }

const (
	defaultQianfanV2Base = "https://qianfan.baidubce.com/v2"
	defaultLegacyBase    = "https://aip.baidubce.com"
	legacyPath           = "/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions"
	v2Path               = "/chat/completions"
)

type qianfanMode int

const (
	modeV2 qianfanMode = iota
	modeLegacy
)

func (c *baiduClient) resolveMode(mc *config.ModelConfig) qianfanMode {
	if t := strings.ToLower(strings.TrimSpace(mc.APIType)); t != "" {
		if t == "legacy" || t == "rpc" {
			return modeLegacy
		}
		if t == "v2" || t == "openai" {
			return modeV2
		}
	}
	base := strings.ToLower(strings.TrimSpace(mc.Endpoint))
	switch {
	case strings.Contains(base, "aip.baidubce.com") || strings.Contains(base, "/rpc/2.0/"):
		return modeLegacy
	case base == "":
		return modeV2
	default:
		if strings.Contains(base, "qianfan") || strings.Contains(base, "/v2") {
			return modeV2
		}
	}
	// 默认走新版
	return modeV2
}

func (c *baiduClient) endpoint(mc *config.ModelConfig, mode qianfanMode) string {
	base := strings.TrimRight(strings.TrimSpace(mc.Endpoint), "/")
	if mode == modeV2 {
		if base == "" {
			base = defaultQianfanV2Base
		} else if strings.HasSuffix(base, "/v1") {
			base = strings.TrimSuffix(base, "/v1") + "/v2"
		} else if !strings.HasSuffix(base, "/v2") && !strings.Contains(base, "/v2/") {
			// 如果包含 qianfan 但没写版本，默认补上 /v2
			if strings.Contains(strings.ToLower(base), "qianfan") {
				base += "/v2"
			}
		}
		return strings.TrimRight(base, "/") + v2Path
	}
	if base == "" || strings.Contains(strings.ToLower(base), "qianfan") {
		base = defaultLegacyBase
	}
	return base + legacyPath
}

func (c *baiduClient) httpClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		to = 60 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (c *baiduClient) makeBody(mc *config.ModelConfig, userMessage string, streaming bool, includeModel bool) ([]byte, error) {
	sys := strings.TrimSpace(mc.SystemPrompt)
	msgs := []map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": userMessage},
	}
	req := map[string]any{
		"messages": msgs,
		"stream":   streaming,
	}
	if includeModel {
		if m := strings.TrimSpace(resolveQianfanModel(mc.Model)); m != "" {
			req["model"] = m
		}
	}
	if mc.Temperature > 0 {
		req["temperature"] = mc.Temperature
	}
	// 部分版本用 max_output_tokens；如你开通的不同可在此处调整
	if mc.MaxTokens > 0 {
		req["max_output_tokens"] = mc.MaxTokens
	}
	// 透传扩展
	for k, v := range mc.Extra {
		if k == "temperature" || k == "max_output_tokens" || k == "stream" || k == "messages" || k == "model" {
			continue
		}
		req[k] = v
	}
	return json.Marshal(req)
}

func resolveQianfanModel(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return ""
	}
	normalized := strings.ToLower(strings.ReplaceAll(m, "_", "-"))
	aliases := map[string]string{
		"ernie-bot-4":      "ernie-4.0-8k",
		"ernie-bot":        "ernie-3.5-8k",
		"ernie-speed-8k":   "ernie-speed-8k",
		"ernie-speed-128k": "ernie-speed-128k",
		"ernie-lite-8k":    "ernie-lite-8k",
		"ernie-tiny-8k":    "ernie-tiny-8k",
	}
	if mapped, ok := aliases[normalized]; ok {
		return mapped
	}
	return m
}

/* ------------------- Invoke（非流式） ------------------- */

func (c *baiduClient) Invoke(ctx context.Context, mc *config.ModelConfig, userMessage string) (string, error) {
	mode := c.resolveMode(mc)
	body, err := c.makeBody(mc, userMessage, false, mode == modeV2)
	if err != nil {
		return "", err
	}

	req, err := c.buildRequest(ctx, mc, body, mode)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		bt, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("baidu invoke status=%d body=%s", resp.StatusCode, string(bt))
	}

	var jr map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return "", err
	}

	// V2：OpenAI 兼容响应
	if ch, ok := jr["choices"].([]any); ok && len(ch) > 0 {
		if first, ok := ch[0].(map[string]any); ok {
			if msg, ok := first["message"].(map[string]any); ok {
				if s, ok := msg["content"].(string); ok && s != "" {
					return s, nil
				}
			}
			if delta, ok := first["delta"].(map[string]any); ok {
				if s, ok := delta["content"].(string); ok && s != "" {
					return s, nil
				}
			}
		}
	}
	// 旧版：result 字段
	if s, ok := jr["result"].(string); ok && s != "" {
		return s, nil
	}
	// 错误字段
	if errMap, ok := jr["error"].(map[string]any); ok {
		msg := ""
		if m, ok := errMap["message"].(string); ok {
			msg = m
		}
		if code, ok := errMap["code"].(string); ok {
			if msg != "" {
				return "", fmt.Errorf("baidu: %s (%s)", msg, code)
			}
			msg = code
		}
		if msg != "" {
			return "", errors.New("baidu: " + msg)
		}
	}
	if em, ok := jr["error_msg"].(string); ok && em != "" {
		return "", errors.New("baidu: " + em)
	}
	if em, ok := jr["msg"].(string); ok && em != "" {
		return "", errors.New("baidu: " + em)
	}
	return "", errors.New("baidu: unexpected response")
}

/* ------------------- Stream（增量） ------------------- */

func (c *baiduClient) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	mode := c.resolveMode(mc)
	body, err := c.makeBody(mc, prompt, true, mode == modeV2)
	if err != nil {
		return "", err
	}

	req, err := c.buildRequest(ctx, mc, body, mode)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		// 让上层决定是否回退到 Invoke；也可以直接返回详细错误
		bt, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("baidu stream status=%d body=%s", resp.StatusCode, string(bt))
	}

	sc := bufio.NewScanner(resp.Body)
	// 放大缓冲，避免长行被截断
	buf := make([]byte, 0, 256*1024)
	sc.Buffer(buf, 2*1024*1024)

	var final strings.Builder
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return final.String(), ctx.Err()
		default:
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		// 兼容 "data: {...}" 前缀
		line = core.TrimDataPrefix(line)

		// [DONE] / 结束标记（不同版本可能不同）
		if bytes.EqualFold(line, []byte("[DONE]")) {
			break
		}

		var chunk map[string]any
		if err := json.Unmarshal(line, &chunk); err != nil {
			// 无法解析本行，略过
			continue
		}
		if err := c.handleStreamChunk(chunk, mode, onDelta, &final); err != nil {
			return final.String(), err
		}
	}
	return final.String(), nil
}

func (c *baiduClient) handleStreamChunk(chunk map[string]any, mode qianfanMode, onDelta func(string), final *strings.Builder) error {
	if mode == modeV2 {
		if errObj, ok := chunk["error"].(map[string]any); ok {
			msg := ""
			if m, ok := errObj["message"].(string); ok {
				msg = m
			}
			if msg == "" {
				msg = "baidu stream error"
			}
			return errors.New(msg)
		}
		if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if delta, ok := choice["delta"].(map[string]any); ok {
					if content, ok := delta["content"].(string); ok && content != "" {
						if onDelta != nil {
							onDelta(content)
						}
						final.WriteString(content)
					}
				}
				if reason, ok := choice["finish_reason"].(string); ok && reason != "" && strings.EqualFold(reason, "stop") {
					return nil
				}
			}
		}
		return nil
	}
	// legacy
	if em, ok := chunk["error_msg"].(string); ok && em != "" {
		return errors.New("baidu: " + em)
	}
	if s, ok := chunk["result"].(string); ok && s != "" {
		if onDelta != nil {
			onDelta(s)
		}
		final.WriteString(s)
	}
	return nil
}

func (c *baiduClient) buildRequest(ctx context.Context, mc *config.ModelConfig, body []byte, mode qianfanMode) (*http.Request, error) {
	url := c.endpoint(mc, mode)
	reqURL := url
	if mode == modeLegacy {
		token := strings.TrimSpace(mc.AccessToken)
		if token == "" {
			token = strings.TrimSpace(mc.APIKey)
		}
		if token == "" {
			return nil, errors.New("baidu: missing access_token or api_key")
		}
		reqURL = url + "?access_token=" + token
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mode == modeV2 {
		token := strings.TrimSpace(mc.APIKey)
		if token == "" {
			token = strings.TrimSpace(mc.AccessToken)
		}
		if token == "" {
			return nil, errors.New("baidu: missing api_key")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if appID := strings.TrimSpace(mc.Organization); appID != "" {
			req.Header.Set("appid", appID)
		}
	}
	return req, nil
}
