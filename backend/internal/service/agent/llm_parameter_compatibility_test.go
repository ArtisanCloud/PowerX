package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/stretchr/testify/require"
)

func TestQuickCallLLMUsesModelSpecificWireParameters(t *testing.T) {
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })

	requests := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		requests <- body
		fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	// 使用表单草稿入口及真实目录合并，不依赖数据库或真实供应商。
	svc := &AgentSettingService{}
	for _, tc := range []struct {
		provider, model, tokenKey string
		sampling                  bool
	}{
		{"openai", "gpt-6-luna", "max_completion_tokens", false},
		{"openai", "gpt-6-astra", "max_completion_tokens", false},
		{"openai", "o3-mini", "max_completion_tokens", false},
		{"openai", "gpt-5.2", "max_completion_tokens", true},
		{"openai", "gpt-4o-mini", "max_tokens", true},
		{"qwen-cn", "qwen-plus", "max_tokens", true},
	} {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			out, err := svc.QuickCallLLM(context.Background(), "dev", nil, tc.provider, tc.model, srv.URL+"/v1", "fixture-key", "", "", "", "", .7, 4096, "hello")
			require.NoError(t, err)
			require.Equal(t, "OK", out)
			body := <-requests
			require.EqualValues(t, 4096, body[tc.tokenKey])
			if tc.sampling {
				require.InDelta(t, .7, body["temperature"], .0001)
			} else {
				require.NotContains(t, body, "temperature")
			}
			require.NotContains(t, body, "api_path")
		})
	}
}
