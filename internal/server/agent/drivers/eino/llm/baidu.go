package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
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

func NewBaiduClient() LLMClient { return &baiduClient{} }

func (c *baiduClient) endpoint(mc *config.ModelConfig) string {
	base := strings.TrimRight(mc.Endpoint, "/")
	if base == "" {
		base = "https://aip.baidubce.com"
	}
	// 你也可以根据 mc.Model 衍生具体子路径，这里统一用 chat/completions
	return base + "/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions"
}

func (c *baiduClient) httpClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		to = 60 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (c *baiduClient) makeBody(mc *config.ModelConfig, userMessage string, streaming bool) ([]byte, error) {
	sys := strings.TrimSpace(mc.SystemPrompt)
	msgs := []map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": userMessage},
	}
	req := map[string]any{
		"messages": msgs,
		"stream":   streaming,
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
		if k == "temperature" || k == "max_output_tokens" || k == "stream" || k == "messages" {
			continue
		}
		req[k] = v
	}
	return json.Marshal(req)
}

func (c *baiduClient) authToken(mc *config.ModelConfig) (string, error) {
	// 优先 AccessToken；回退 APIKey
	token := strings.TrimSpace(mc.AccessToken)
	if token == "" {
		token = strings.TrimSpace(mc.APIKey)
	}
	if token == "" {
		return "", errors.New("baidu: missing access_token or api_key")
	}
	return token, nil
}

/* ------------------- Invoke（非流式） ------------------- */

func (c *baiduClient) Invoke(ctx context.Context, mc *config.ModelConfig, userMessage string) (string, error) {
	token, err := c.authToken(mc)
	if err != nil {
		return "", err
	}
	body, err := c.makeBody(mc, userMessage, false)
	if err != nil {
		return "", err
	}

	url := c.endpoint(mc) + "?access_token=" + token
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

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

	// 常见：直接 result
	if s, ok := jr["result"].(string); ok && s != "" {
		return s, nil
	}
	// 兼容：choices[0].message.content
	if ch, ok := jr["choices"].([]any); ok && len(ch) > 0 {
		if first, ok := ch[0].(map[string]any); ok {
			if msg, ok := first["message"].(map[string]any); ok {
				if s, ok := msg["content"].(string); ok && s != "" {
					return s, nil
				}
			}
		}
	}
	// 错误字段（不同版本可能是 error_code / error_msg）
	if em, ok := jr["error_msg"].(string); ok && em != "" {
		return "", errors.New("baidu: " + em)
	}
	return "", errors.New("baidu: unexpected response")
}

/* ------------------- Stream（增量） ------------------- */

func (c *baiduClient) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	token, err := c.authToken(mc)
	if err != nil {
		return "", err
	}
	body, err := c.makeBody(mc, prompt, true)
	if err != nil {
		return "", err
	}

	url := c.endpoint(mc) + "?access_token=" + token
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

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
		line = TrimDataPrefix(line)

		// [DONE] / 结束标记（不同版本可能不同）
		if bytes.EqualFold(line, []byte("[DONE]")) {
			break
		}

		var chunk map[string]any
		if err := json.Unmarshal(line, &chunk); err != nil {
			// 无法解析本行，略过
			continue
		}
		// 错误字段
		if em, ok := chunk["error_msg"].(string); ok && em != "" {
			return final.String(), errors.New("baidu: " + em)
		}
		// 增量内容字段（常见：result）
		if s, ok := chunk["result"].(string); ok && s != "" {
			if onDelta != nil {
				onDelta(s)
			}
			final.WriteString(s)
		}
		// 结束字段（is_end / done）
		if done, ok := chunk["is_end"].(bool); ok && done {
			break
		}
		if done, ok := chunk["done"].(bool); ok && done {
			break
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return final.String(), err
	}
	return final.String(), nil
}
