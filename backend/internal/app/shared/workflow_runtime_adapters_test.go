package shared

import "testing"

func TestExtractKnowledgeContentRequiresDeclaredContent(t *testing.T) {
	got := extractKnowledgeContent(map[string]any{
		"metadata": map[string]any{
			"title": "no declared content",
		},
	})
	if got != "" {
		t.Fatalf("expected no content fallback, got %q", got)
	}
}

func TestExtractKnowledgeContentReadsDraftItems(t *testing.T) {
	got := extractKnowledgeContent(map[string]any{
		"draft_items": []interface{}{
			map[string]interface{}{
				"content": "营销方法论草稿",
			},
		},
	})
	if got != "营销方法论草稿" {
		t.Fatalf("expected draft item content, got %q", got)
	}
}
