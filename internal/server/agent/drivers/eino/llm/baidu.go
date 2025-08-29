package llm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// services/agent/drivers/eino/llm/baidu.go

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"
)

type baiduClient struct{}

func (c *baiduClient) ChatOnce(ctx context.Context, mc ModelConfig, userMessage string) (string, error) {
	// 简化：优先用 access_token；也兼容把 api_key 当 access_token 传入
	token := mc.AccessToken
	if token == "" && mc.APIKey != "" {
		token = mc.APIKey
	}
	if token == "" {
		return "", errors.New("baidu: missing access_token")
	}

	if mc.Model == "" {
		mc.Model = "chat/completions"
	} // 实际以你的开通接口为准

	url := strings.TrimRight(mc.Endpoint, "/") + "/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions?access_token=" + token
	body := map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": mc.SystemPrompt},
			{"role": "user", "content": userMessage},
		},
		"temperature": mc.Temperature,
		// "max_output_tokens": mc.MaxTokens, // 具体字段以你开通版本为准
	}
	bs, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bs))
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		bt, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("baidu status=%d body=%s", resp.StatusCode, string(bt))
	}

	var jr map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return "", err
	}

	// 常见字段 result
	if s, ok := jr["result"].(string); ok && s != "" {
		return s, nil
	}
	// 兼容 choices[0].message.content
	if ch, ok := jr["choices"].([]any); ok && len(ch) > 0 {
		if first, ok := ch[0].(map[string]any); ok {
			if msg, ok := first["message"].(map[string]any); ok {
				if s, ok := msg["content"].(string); ok {
					return s, nil
				}
			}
		}
	}
	return "", errors.New("baidu: unexpected response")
}
