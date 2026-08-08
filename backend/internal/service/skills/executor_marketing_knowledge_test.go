package skills

import (
	"context"
	"testing"
)

func TestMarketingKnowledgeExecutorParseAndExtract(t *testing.T) {
	exec := newMarketingKnowledgeExecutor()
	parseInput := ExecuteInput{
		SkillID: "marketing.audio_or_document_parse",
		Version: "1.0.0",
		TraceID: "trace-marketing",
		Payload: map[string]interface{}{
			"source": map[string]interface{}{
				"type":     "text",
				"content":  "新品发布复盘：高意向客户识别不足导致转化延迟。",
				"context":  "活动复盘",
				"language": "zh",
			},
		},
	}
	if !exec.CanHandle(parseInput) {
		t.Fatal("expected marketing parser to be handled")
	}
	parsed, err := exec.Execute(context.Background(), parseInput)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed["content"] == "" {
		t.Fatal("expected parsed content")
	}

	extractInput := ExecuteInput{
		SkillID: "marketing.extract_methodology",
		Version: "1.0.0",
		TraceID: "trace-marketing",
		Payload: parsed,
	}
	extracted, err := exec.Execute(context.Background(), extractInput)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if extracted["summary"] == "" {
		t.Fatal("expected extracted summary")
	}
	items, ok := extracted["draft_items"].([]map[string]interface{})
	if !ok || len(items) == 0 {
		t.Fatal("expected draft_items")
	}
}

func TestMarketingKnowledgeExecutorRequiresStructuredSource(t *testing.T) {
	exec := newMarketingKnowledgeExecutor()
	_, err := exec.Execute(context.Background(), ExecuteInput{
		SkillID: "marketing.audio_or_document_parse",
		Payload: map[string]interface{}{
			"type":    "text",
			"content": "flat payload must not be accepted as a source contract",
		},
	})
	if err == nil {
		t.Fatal("expected missing source object error")
	}

	_, err = exec.Execute(context.Background(), ExecuteInput{
		SkillID: "marketing.audio_or_document_parse",
		Payload: map[string]interface{}{
			"source": map[string]interface{}{
				"type": "text",
			},
		},
	})
	if err == nil {
		t.Fatal("expected missing source.content error")
	}
}
