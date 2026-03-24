package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
)

func TestOpenAIVLMStreamSSE(t *testing.T) {
	var sawImageURL bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Fatalf("missing authorization header")
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if stream, _ := req["stream"].(bool); !stream {
			t.Fatalf("expected stream=true")
		}
		msgs, ok := req["messages"].([]any)
		if !ok || len(msgs) < 2 {
			t.Fatalf("unexpected messages: %+v", req["messages"])
		}
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			content, _ := msg["content"].([]any)
			for _, c := range content {
				part, _ := c.(map[string]any)
				if strings.TrimSpace(asString(part["type"])) == "image_url" {
					sawImageURL = true
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client := NewVLMClient()
	mc := &config.ModelConfig{
		Provider: "openai",
		Endpoint: srv.URL,
		APIKey:   "test-key",
		Model:    "gpt-4.1-mini",
	}
	in := contract.VLMRequest{
		Messages: []contract.Message{
			{
				Role: "system",
				Content: []contract.ContentPart{
					{Type: contract.ContentTypeText, Text: "你是图像分析助手"},
				},
			},
			{
				Role: "user",
				Content: []contract.ContentPart{
					{Type: contract.ContentTypeImageURL, URL: "https://example.com/a.png"},
					{Type: contract.ContentTypeText, Text: "请描述图片"},
				},
			},
		},
		Runtime: map[string]any{"config": mc},
	}

	deltas := make([]string, 0, 2)
	resp, err := client.Stream(context.Background(), in, func(s string) {
		deltas = append(deltas, s)
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp == nil || resp.Text != "你好" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(deltas) != 2 || deltas[0] != "你" || deltas[1] != "好" {
		t.Fatalf("unexpected deltas: %+v", deltas)
	}
	if !sawImageURL {
		t.Fatalf("expected image_url part in request")
	}
}

func TestOpenAIVLMStreamFallbackToInvoke(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		stream, _ := req["stream"].(bool)

		if current == 1 && stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"note":"non-sse response"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fallback"}}]}`))
	}))
	defer srv.Close()

	client := NewVLMClient()
	mc := &config.ModelConfig{
		Provider: "openai",
		Endpoint: srv.URL,
		APIKey:   "test-key",
		Model:    "gpt-4.1-mini",
	}
	in := contract.VLMRequest{
		Messages: []contract.Message{
			{
				Role: "user",
				Content: []contract.ContentPart{
					{Type: contract.ContentTypeText, Text: "hello"},
				},
			},
		},
		Runtime: map[string]any{"config": mc},
	}

	resp, err := client.Stream(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp == nil || resp.Text != "fallback" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Fatalf("expected 2 calls (stream + invoke fallback), got %d", callCount)
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
