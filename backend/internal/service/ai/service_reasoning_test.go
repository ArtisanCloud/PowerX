package ai

import (
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
)

func TestParseReasoningConfig(t *testing.T) {
	cfg := parseReasoningConfig(map[string]interface{}{
		"thinking":        true,
		"enable_thinking": false,
		"thinking_budget": 512,
		"reasoning": map[string]interface{}{
			"enabled": true,
			"effort":  "high",
			"expose":  "summary",
		},
	})
	if !cfg.Enabled {
		t.Fatalf("expected enabled=true")
	}
	if cfg.Effort != "high" {
		t.Fatalf("expected effort=high, got %s", cfg.Effort)
	}
	if cfg.Expose != "summary" {
		t.Fatalf("expected expose=summary, got %s", cfg.Expose)
	}
	if cfg.ProviderThink == nil || *cfg.ProviderThink {
		t.Fatalf("expected enable_thinking=false passthrough")
	}
	if cfg.Budget != 512 {
		t.Fatalf("expected thinking_budget=512, got %d", cfg.Budget)
	}
}

func TestApplyReasoningConfig(t *testing.T) {
	mc := &aiconfig.ModelConfig{Extra: map[string]any{}}
	applyReasoningConfig("openai", mc, map[string]interface{}{
		"reasoning": map[string]interface{}{
			"enabled": true,
			"effort":  "medium",
			"expose":  "full",
		},
	})
	if _, ok := mc.Extra["reasoning"]; !ok {
		t.Fatalf("expected reasoning passthrough for openai")
	}
	if mc.Extra["reasoning_expose"] != "full" {
		t.Fatalf("expected reasoning_expose=full, got %+v", mc.Extra["reasoning_expose"])
	}

	other := &aiconfig.ModelConfig{Extra: map[string]any{}}
	applyReasoningConfig("ollama", other, map[string]interface{}{
		"reasoning": map[string]interface{}{"enabled": true},
	})
	if len(other.Extra) != 0 {
		t.Fatalf("expected no passthrough for unsupported provider, got %+v", other.Extra)
	}
}

func TestApplyReasoningConfig_DisableThinkingByProvider(t *testing.T) {
	mc := &aiconfig.ModelConfig{Extra: map[string]any{}}
	applyReasoningConfig("qwen-cn", mc, map[string]interface{}{
		"thinking": false,
	})
	if mc.Extra["enable_thinking"] != false {
		t.Fatalf("expected enable_thinking=false, got %+v", mc.Extra["enable_thinking"])
	}
	if _, ok := mc.Extra["reasoning"]; ok {
		t.Fatalf("did not expect reasoning payload when thinking=false")
	}
}

func TestApplyReasoningConfig_OllamaThinkingSwitch(t *testing.T) {
	mc := &aiconfig.ModelConfig{Extra: map[string]any{}}
	applyReasoningConfig("ollama", mc, map[string]interface{}{
		"thinking": false,
	})
	if mc.Extra["think"] != false {
		t.Fatalf("expected think=false, got %+v", mc.Extra["think"])
	}
	if mc.Extra["thinking"] != false {
		t.Fatalf("expected thinking=false, got %+v", mc.Extra["thinking"])
	}
}

func TestApplyReasoningConfig_QwenIntlThinkingSwitchAndBudget(t *testing.T) {
	mc := &aiconfig.ModelConfig{Extra: map[string]any{}}
	applyReasoningConfig("qwen-intl", mc, map[string]interface{}{
		"thinking":        false,
		"thinking_budget": 256,
	})
	if mc.Extra["enable_thinking"] != false {
		t.Fatalf("expected enable_thinking=false, got %+v", mc.Extra["enable_thinking"])
	}
	if mc.Extra["thinking_budget"] != 256 {
		t.Fatalf("expected thinking_budget=256, got %+v", mc.Extra["thinking_budget"])
	}
}

func TestStripThinkingTags(t *testing.T) {
	in := "<think>\nprivate chain\n</think>\n\n{\"intent\":\"start_new_design\",\"confidence\":0.95}"
	out := stripThinkingTags(in)
	if out != "{\"intent\":\"start_new_design\",\"confidence\":0.95}" {
		t.Fatalf("unexpected stripped output: %q", out)
	}
}

func TestBuildLLMPrompts(t *testing.T) {
	system, user := BuildLLMPrompts([]ContentItem{
		{Role: "system", Type: "text", Content: "你是编辑助手"},
		{Role: "user", Type: "text", Content: "请改写这段文案"},
		{Type: "text", Content: "并给三个版本"},
	})
	if system != "你是编辑助手" {
		t.Fatalf("unexpected system prompt: %q", system)
	}
	if user != "请改写这段文案\n并给三个版本" {
		t.Fatalf("unexpected user prompt: %q", user)
	}
}

func TestBuildVLMMessageKeepsRoleGrouping(t *testing.T) {
	msgs := buildVLMMessage([]ContentItem{
		{Role: "system", Type: "text", Content: "你是视觉分析助手"},
		{Role: "user", Type: "image_url", URL: "https://example.com/a.png"},
		{Role: "user", Type: "text", Content: "请描述这张图"},
		{Role: "assistant", Type: "text", Content: "先确认主体"},
		{Type: "text", Content: "继续补充细节"},
	})
	if len(msgs) != 4 {
		t.Fatalf("unexpected message count: %d", len(msgs))
	}
	if msgs[0].Role != "system" || len(msgs[0].Content) != 1 || msgs[0].Content[0].Text != "你是视觉分析助手" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "user" || len(msgs[1].Content) != 2 {
		t.Fatalf("unexpected second message: %+v", msgs[1])
	}
	if msgs[1].Content[0].Type != contract.ContentTypeImageURL || msgs[1].Content[1].Type != contract.ContentTypeText {
		t.Fatalf("unexpected user content parts: %+v", msgs[1].Content)
	}
	if msgs[2].Role != "assistant" || len(msgs[2].Content) != 1 {
		t.Fatalf("unexpected third message: %+v", msgs[2])
	}
	if msgs[3].Role != "user" || len(msgs[3].Content) != 1 || msgs[3].Content[0].Text != "继续补充细节" {
		t.Fatalf("unexpected fourth message: %+v", msgs[3])
	}
}

func TestBuildVLMMessageFallback(t *testing.T) {
	msgs := buildVLMMessage([]ContentItem{
		{Role: "system", Type: "image_url", URL: ""},
		{Role: "user", Type: "text", Content: "   "},
	})
	if len(msgs) != 1 {
		t.Fatalf("unexpected message count: %d", len(msgs))
	}
	if msgs[0].Role != "user" || len(msgs[0].Content) != 1 {
		t.Fatalf("unexpected fallback message: %+v", msgs[0])
	}
	if msgs[0].Content[0].Type != contract.ContentTypeText || msgs[0].Content[0].Text != "Describe the image." {
		t.Fatalf("unexpected fallback content: %+v", msgs[0].Content[0])
	}
}
