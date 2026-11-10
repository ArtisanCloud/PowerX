package ai

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// normalizeVisionInputs 把 Base64 图片转换为标准 data URL，绝不当作提示词文本。
func normalizeVisionInputs(inputs []ContentItem) ([]ContentItem, error) {
	result := append([]ContentItem(nil), inputs...)
	for i := range result {
		item := &result[i]
		if item.Type != "image_base64" {
			continue
		}
		value := strings.TrimSpace(item.Content)
		if value == "" {
			value = strings.TrimSpace(item.URL)
		}
		mime, encoded := "", value
		if strings.HasPrefix(value, "data:") {
			header, body, found := strings.Cut(value, ",")
			if !found || !strings.HasSuffix(header, ";base64") {
				return nil, MediaError(422, "AI_INPUT_INVALID")
			}
			mime, encoded = strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64"), body
		}
		if len(encoded) > (20<<20)*4/3+4 {
			return nil, MediaError(422, "AI_INPUT_LIMIT_EXCEEDED")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) == 0 {
			return nil, MediaError(422, "AI_INPUT_INVALID")
		}
		if len(data) > 20<<20 {
			return nil, MediaError(422, "AI_INPUT_LIMIT_EXCEEDED")
		}
		detected := http.DetectContentType(data)
		switch detected {
		case "image/png", "image/jpeg", "image/webp", "image/gif":
		default:
			return nil, MediaError(422, "AI_INPUT_INVALID")
		}
		if mime != "" && mime != detected {
			return nil, MediaError(422, "AI_INPUT_INVALID")
		}
		item.Type, item.Content, item.URL = "image_url", "", "data:"+detected+";base64,"+base64.StdEncoding.EncodeToString(data)
	}
	return result, nil
}
