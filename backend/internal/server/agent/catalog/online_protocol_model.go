package catalog

const OnlineUsageSelectedTag = "online-usage-selected"

// ProtocolModel 为在线型号选择现有驱动合同。它不登记型号，也不声称供应商已验证该用途。
// 型号存在性由租户在线目录校验；本地只维护固定协议、参数和输入的约束。
func (r *Registry) ProtocolModel(modality, provider, app, id string) *ModelManifest {
	models, _ := r.ModelsByApp(modality, provider, app)
	for _, model := range models {
		if model.ID == id {
			copy := model
			return &copy
		}
	}
	manifest, ok := r.Manifest(provider)
	if !ok || manifest.Drivers[modality] == "" {
		return nil
	}
	model := &ModelManifest{ID: id, Label: id, Tags: []string{"online", "protocol-template"}, Defaults: map[string]any{}, Operations: map[string]MediaOperation{}}
	driver := manifest.Drivers[modality]
	number := func(kind string, min, max float64) ParameterSchema {
		return ParameterSchema{Type: kind, Minimum: &min, Maximum: &max}
	}
	op := MediaOperation{Driver: driver, InputTypes: []string{"text"}, Parameters: map[string]ParameterSchema{}}
	switch modality {
	case "vlm":
		if driver != "openai" && driver != "qwen" {
			return nil
		}
		op.InputTypes, op.MaxImages = []string{"text", "image_url", "image_base64"}, 1
		op.Parameters = map[string]ParameterSchema{"max_tokens": number("integer", 1, 128000), "temperature": number("number", 0, 2), "top_p": number("number", 0, 1), "json_mode": {Type: "boolean"}}
		model.Defaults["max_tokens"] = 2048
		model.Operations[VisionUnderstanding] = op
	case "image":
		if driver != "openai" && driver != "qwen" {
			return nil
		}
		op.Parameters = map[string]ParameterSchema{"n": number("integer", 1, 10), "size": {Type: "string"}, "quality": {Type: "string"}, "format": {Type: "string", Enum: []any{"png", "jpeg", "webp"}}}
		model.Operations[ImageGeneration] = op
	case "audio_tts":
		if driver != "openai" && driver != "qwen" {
			return nil
		}
		op.Parameters = map[string]ParameterSchema{"voice": {Type: "string"}, "speed": number("number", 0.25, 4), "format": {Type: "string"}}
		model.Operations[SpeechSynthesis] = op
	default:
		// 文本、嵌入等使用已有模态驱动。没有驱动时上方已经拒绝。
	}
	return model
}
