package skills

import (
	"context"
	"fmt"
	"strings"
)

type marketingKnowledgeExecutor struct{}

func newMarketingKnowledgeExecutor() SkillExecutor { return &marketingKnowledgeExecutor{} }

func (e *marketingKnowledgeExecutor) CanHandle(in ExecuteInput) bool {
	skillID := strings.ToLower(strings.TrimSpace(in.SkillID))
	return skillID == "marketing.audio_or_document_parse" || skillID == "marketing.extract_methodology"
}

func (e *marketingKnowledgeExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(in.SkillID)) {
	case "marketing.audio_or_document_parse":
		return e.parseSource(in)
	case "marketing.extract_methodology":
		return e.extractMethodology(in)
	default:
		return nil, fmt.Errorf("unsupported marketing skill: %s", in.SkillID)
	}
}

func (e *marketingKnowledgeExecutor) parseSource(in ExecuteInput) (map[string]interface{}, error) {
	source := mapFromAny(in.Payload["source"])
	if len(source) == 0 {
		return nil, fmt.Errorf("source object is required")
	}
	sourceType := strings.ToLower(strings.TrimSpace(extractString(source, "type", "source_type")))
	if sourceType == "" {
		return nil, fmt.Errorf("source.type is required")
	}
	contextText := strings.TrimSpace(extractString(source, "context"))
	language := strings.TrimSpace(extractString(source, "language"))
	if language == "" {
		language = "zh"
	}
	out := map[string]interface{}{
		"skill_id":    in.SkillID,
		"version":     in.Version,
		"trace_id":    in.TraceID,
		"source_type": sourceType,
		"context":     contextText,
		"language":    language,
		"parsed_at":   "workflow_runtime",
		"source":      source,
	}
	switch sourceType {
	case "text":
		content := strings.TrimSpace(extractString(source, "content", "text"))
		if content == "" {
			return nil, fmt.Errorf("source.content is required for text source")
		}
		out["content"] = content
	case "link":
		url := strings.TrimSpace(extractString(source, "url", "link"))
		if url == "" {
			return nil, fmt.Errorf("source.url is required for link source")
		}
		out["content"] = fmt.Sprintf("link:%s", url)
		out["url"] = url
	case "audio", "document":
		assetUUID := strings.TrimSpace(extractString(source, "asset_uuid", "asset_ref"))
		if assetUUID == "" {
			return nil, fmt.Errorf("source.asset_uuid is required for %s source", sourceType)
		}
		out["content"] = fmt.Sprintf("%s asset:%s", sourceType, assetUUID)
		out["asset_uuid"] = assetUUID
	default:
		return nil, fmt.Errorf("unsupported source.type: %s", sourceType)
	}
	return out, nil
}

func (e *marketingKnowledgeExecutor) extractMethodology(in ExecuteInput) (map[string]interface{}, error) {
	content := strings.TrimSpace(extractString(in.Payload, "content", "text", "rendered_text"))
	if content == "" {
		return nil, fmt.Errorf("content is required for marketing.extract_methodology")
	}
	contextText := strings.TrimSpace(extractString(in.Payload, "context"))
	if contextText == "" {
		contextText = strings.TrimSpace(extractString(in.Context, "context"))
	}
	observation := firstSentence(content)
	method := "marketing_methodology"
	if contextText != "" {
		method = strings.ToLower(strings.ReplaceAll(contextText, " ", "_"))
	}
	draftItems := []map[string]interface{}{
		{
			"type":     "observation",
			"title":    "marketing_observation",
			"content":  observation,
			"evidence": content,
		},
		{
			"type":    "method",
			"title":   method,
			"content": content,
		},
	}
	return map[string]interface{}{
		"skill_id":    in.SkillID,
		"version":     in.Version,
		"trace_id":    in.TraceID,
		"context":     contextText,
		"content":     content,
		"summary":     observation,
		"draft_items": draftItems,
		"methodology": map[string]interface{}{
			"observation": observation,
			"method":      method,
			"source_type": extractString(in.Payload, "source_type"),
		},
	}, nil
}

func mapFromAny(raw interface{}) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	if value, ok := raw.(map[string]interface{}); ok {
		return value
	}
	return map[string]interface{}{}
}

func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, sep := range []string{"。", "；", ";", "\n", "."} {
		if idx := strings.Index(text, sep); idx > 0 {
			return strings.TrimSpace(text[:idx])
		}
	}
	return text
}
