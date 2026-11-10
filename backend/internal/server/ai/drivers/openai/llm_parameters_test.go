package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/stretchr/testify/require"
)

func TestChatParametersByProviderModelAndEffort(t *testing.T) {
	cases := []struct {
		provider, model, effort string
		modern, sampling        bool
	}{
		{"openai", "gpt-4o", "", false, true},
		{"openai", "gpt-4o-mini", "", false, true},
		{"openai", "gpt-4.1-mini", "", false, true},
		{"openai", "gpt-3.5-turbo", "", false, true},
		{"openai", "o1", "", true, false},
		{"openai", "o3-mini-2025-01-31", "low", true, false},
		{"openai", "o4-mini", "high", true, false},
		{"openai", "gpt-5", "", true, false},
		{"openai", "gpt-5-mini", "", true, false},
		{"openai", "gpt-5-nano", "minimal", true, false},
		{"openai", "gpt-5.1", "", true, true},
		{"openai", "gpt-5.2-2025-12-11", "", true, true},
		{"openai", "gpt-5.2", "high", true, false},
		{"openai", "gpt-5.4", "none", true, true},
		{"openai", "gpt-5.4-mini", "high", true, false},
		{"openai", "gpt-5.5", "", true, false},
		{"openai", "gpt-5.5", "none", true, true},
		{"openai", "gpt-5.4-pro", "high", true, false},
		{"openai", "gpt-5.6", "", true, false},
		{"openai", "gpt-6-luna", "", true, false},
		{"openai", "gpt-6-luna-2026-08-01", "none", true, true},
		{"openai", "gpt-6-sol", "medium", true, false},
		{"openai", "gpt-6-sol", "none", true, true},
		{"openai", "gpt-6.1-terra", "none", true, true},
		{"openai", "gpt-6-astra", "", true, false},
		{"openai", "gpt-6.1-sol", "high", true, false},
		{"qwen-cn", "qwen-plus", "", false, true},
		{"deepseek", "deepseek-chat", "", false, true},
		{"openrouter", "openai/gpt-6-luna", "", false, true},
		{"vllm", "gpt-6-luna", "", false, true},
		{"openai", "custom-model", "", false, true},
	}
	for _, tc := range cases {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/%s/stream=%t", tc.provider, tc.model, tc.effort, stream), func(t *testing.T) {
				extra := map[string]any{"api_path": "/v1/chat/completions", "base_url": "https://example.invalid", "custom_option": true}
				if tc.effort != "" {
					extra["reasoning_effort"] = tc.effort
				}
				mc := &config.ModelConfig{Provider: tc.provider, Model: tc.model, Temperature: .7, TopP: .9, MaxTokens: 4096, Extra: extra}
				body, err := NewLLMClient(tc.provider).makeBody(mc, "hello", stream)
				require.NoError(t, err)
				var request map[string]any
				require.NoError(t, json.Unmarshal(body, &request))
				tokenKey, omittedKey := "max_tokens", "max_completion_tokens"
				if tc.modern {
					tokenKey, omittedKey = omittedKey, tokenKey
				}
				require.EqualValues(t, 4096, request[tokenKey])
				require.NotContains(t, request, omittedKey)
				if tc.sampling {
					require.InDelta(t, .7, request["temperature"], .0001)
					require.InDelta(t, .9, request["top_p"], .0001)
				} else {
					require.NotContains(t, request, "temperature")
					require.NotContains(t, request, "top_p")
				}
				if tc.effort == "" {
					require.NotContains(t, request, "reasoning_effort", "不能擅自切换模型的默认推理强度")
				} else {
					require.Equal(t, tc.effort, request["reasoning_effort"])
				}
				require.Equal(t, true, request["custom_option"])
				require.NotContains(t, request, "api_path")
				require.NotContains(t, request, "base_url")
				require.Contains(t, mc.Extra, "api_path", "不能修改共享配置")
			})
		}
	}
}

func TestChatParameterExtrasAndExplicitZero(t *testing.T) {
	for _, model := range []string{"gpt-4o", "gpt-6-luna"} {
		body, err := NewLLMClient("openai").makeBody(&config.ModelConfig{Model: model, TemperatureSet: true, Extra: map[string]any{"reasoning_effort": "none"}}, "hello", false)
		require.NoError(t, err)
		var request map[string]any
		require.NoError(t, json.Unmarshal(body, &request))
		require.EqualValues(t, 0, request["temperature"])
	}
	mc := &config.ModelConfig{Model: "gpt-6-luna", MaxTokens: 4096, Extra: map[string]any{
		"max_tokens": 512, "max_completion_tokens": 1024, "temperature": .2, "top_p": .5,
		"logprobs": true, "top_logprobs": 2,
	}}
	body, err := NewLLMClient("openai").makeBody(mc, "hello", false)
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, json.Unmarshal(body, &request))
	require.EqualValues(t, 1024, request["max_completion_tokens"])
	for _, key := range []string{"max_tokens", "temperature", "top_p", "logprobs", "top_logprobs"} {
		require.NotContains(t, request, key)
		require.Contains(t, mc.Extra, key)
	}
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-5-mini", "o3"} {
		_, err := NewLLMClient("openai").makeBody(&config.ModelConfig{Model: model, Extra: map[string]any{"reasoning_effort": "none"}}, "hello", false)
		require.ErrorContains(t, err, "does not support reasoning_effort=none")
	}
}

// 通过真实 HTTP 编解码检查两条驱动入口，避免只验证策略函数而遗漏实际发出的请求。
func TestReasoningLLMInvokeAndStreamWireParameters(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				requests <- body
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`)
			}))
			defer srv.Close()
			mc := &config.ModelConfig{Provider: "openai", Model: "gpt-6-luna", Endpoint: srv.URL, APIKey: "fixture-key", Temperature: .7, MaxTokens: 4096, Timeout: time.Second}
			client := NewLLMClient("openai")
			if stream {
				var deltas string
				out, err := client.Stream(context.Background(), mc, "hello", func(delta string) { deltas += delta })
				require.NoError(t, err)
				require.Equal(t, "OK", out)
				require.Equal(t, "OK", deltas)
			} else {
				out, err := client.Invoke(context.Background(), mc, "hello")
				require.NoError(t, err)
				require.Equal(t, "OK", out.Text)
			}
			body := <-requests
			require.EqualValues(t, 4096, body["max_completion_tokens"])
			require.NotContains(t, body, "max_tokens")
			require.NotContains(t, body, "temperature")
			require.Empty(t, requests, "参数适配不应触发供应商重试")
		})
	}
}
