package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	_ "golang.org/x/image/webp"
)

const (
	ImageReferenceMaxImages  = 16
	ImageReferenceMaxBytes   = 5 << 20
	ImageReferenceTotalBytes = 20 << 20
	ImageReferenceMaxPixels  = 36000000
)

func imageInputError(code, reason string) *dto.AppError {
	return &dto.AppError{HTTPCode: 422, Code: code, Message: code, Details: map[string]any{"reason": reason}}
}

// ImageReferenceLimit 返回驱动已实现的通道上限；配置只能收紧，不能扩大。
// 自定义生成路径不会被猜测为编辑路径，必须显式提供 image_edit_api_path。
func ImageReferenceLimit(mc *config.ModelConfig) (int, error) {
	if mc == nil || mc.AzureDeployment != "" {
		return 0, nil
	}
	switch strings.TrimSpace(mc.Model) {
	case "gpt-image-2", "gpt-image-2-2026-04-21", "chatgpt-image-latest":
	default:
		return 0, nil
	}
	if _, err := imageEditPath(mc); err != nil {
		return 0, err
	}
	limit := ImageReferenceMaxImages
	if value, ok := mc.Extra["image_edit_max_images"]; ok {
		raw, err := json.Marshal(value)
		if err != nil || json.Unmarshal(raw, &limit) != nil || limit < 0 || limit > ImageReferenceMaxImages {
			return 0, imageInputError("AI_MODEL_CONFIGURATION_INVALID", "invalid_image_edit_max_images")
		}
	}
	return limit, nil
}

func imageEditPath(mc *config.ModelConfig) (string, error) {
	path := "/v1/images/edits"
	if value, exists := mc.Extra["image_edit_api_path"]; exists {
		var ok bool
		path, ok = value.(string)
		if !ok || strings.TrimSpace(path) != path || path == "" {
			return "", imageInputError("AI_MODEL_CONFIGURATION_INVALID", "invalid_image_edit_api_path")
		}
	} else if resolveImageAPIPath(mc) != defaultOpenAIImagePath {
		return "", imageInputError("AI_INPUT_UNSUPPORTED", "channel_has_no_image_edit_binding")
	}
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || strings.ContainsAny(path, "%\\{}*:?#") {
		return "", imageInputError("AI_MODEL_CONFIGURATION_INVALID", "invalid_image_edit_api_path")
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return "", imageInputError("AI_MODEL_CONFIGURATION_INVALID", "invalid_image_edit_api_path")
		}
	}
	return path, nil
}

// Validate 不执行网络请求；与 Generate 共用，保证坏图在登记任务前被拒绝。
func (c *imageClient) Validate(in contract.ImageRequest) error {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return err
	}
	if mc == nil {
		return imageInputError("AI_MODEL_CONFIGURATION_INVALID", "missing_model_config")
	}
	_, err = prepareImageReferences(in, mc)
	return err
}

func prepareImageReferences(in contract.ImageRequest, mc *config.ModelConfig) ([]openAIImageReference, error) {
	if err := validateImageParameters(in, mc); err != nil {
		return nil, err
	}
	if len(in.RefImages) == 0 {
		return nil, nil
	}
	limit, err := ImageReferenceLimit(mc)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		return nil, imageInputError("AI_INPUT_UNSUPPORTED", "channel_has_no_reference_images")
	}
	if len(in.RefImages) > limit {
		return nil, imageInputError("AI_INPUT_LIMIT_EXCEEDED", "too_many_images")
	}
	total := 0
	references := make([]openAIImageReference, 0, len(in.RefImages))
	for index, part := range in.RefImages {
		source, size, err := validateImageReference(part)
		if err != nil {
			if failure, ok := err.(*dto.AppError); ok {
				failure.Details["image_index"] = index
			}
			return nil, err
		}
		total += size
		if total > ImageReferenceTotalBytes {
			return nil, imageInputError("AI_INPUT_LIMIT_EXCEEDED", "total_image_bytes_exceeded")
		}
		references = append(references, openAIImageReference{URL: source})
	}
	return references, nil
}

func validateImageParameters(in contract.ImageRequest, mc *config.ModelConfig) error {
	// 与正式 GPT Image 2 目录一致；保留驱动对历史 standard/hd 的兼容映射。
	if !imageOneOf(strings.TrimSpace(mc.Model), "gpt-image-2", "gpt-image-2-2026-04-21", "chatgpt-image-latest") {
		return nil
	}
	if value := in.Quality; value != "" && !imageOneOf(normalizeImageQuality(value, mc.Model), "auto", "low", "medium", "high") {
		return imageParameterError("quality")
	}
	if in.Size != "" && !imageOneOf(in.Size, "auto", "1024x1024", "1536x1024", "1024x1536") {
		return imageParameterError("size")
	}
	if in.Format != "" && !imageOneOf(normalizeImageFormat(in.Format), "png", "jpeg", "webp") {
		return imageParameterError("format")
	}
	if value, ok := in.Runtime["n"]; ok {
		var n int
		raw, err := json.Marshal(value)
		if err != nil || json.Unmarshal(raw, &n) != nil || n < 1 || n > 10 {
			return imageParameterError("n")
		}
	}
	for _, field := range []string{"response_format", "input_fidelity"} {
		if _, ok := in.Runtime[field]; ok {
			return imageParameterError(field)
		}
	}
	if value, ok := in.Runtime["background"]; ok {
		if text, ok := value.(string); !ok || !imageOneOf(text, "auto", "opaque", "transparent") {
			return imageParameterError("background")
		}
	}
	if value, ok := in.Runtime["output_compression"]; ok {
		var n int
		raw, err := json.Marshal(value)
		if err != nil || json.Unmarshal(raw, &n) != nil || n < 0 || n > 100 || !imageOneOf(normalizeImageFormat(in.Format), "jpeg", "webp") {
			return imageParameterError("output_compression")
		}
	}
	return nil
}

func imageOneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
func imageParameterError(field string) *dto.AppError {
	err := imageInputError("AI_PARAMETER_INVALID", "invalid_image_parameter")
	err.Details["parameter"] = field
	return err
}

// 首版只接受可在提交前完整验证的内嵌图片，不由 Core 随意抓取远程 URL。
func validateImageReference(part contract.ContentPart) (string, int, error) {
	value := strings.TrimSpace(part.URL)
	if part.Type == contract.ContentTypeImageBase64 {
		value = strings.TrimSpace(part.Text)
	}
	if part.Type != contract.ContentTypeImageURL && part.Type != contract.ContentTypeImageBase64 {
		return "", 0, imageInputError("AI_INPUT_UNSUPPORTED", "unsupported_image_input")
	}
	header, encoded, found := strings.Cut(value, ",")
	mime := ""
	if strings.HasPrefix(value, "data:") {
		if !found || !strings.HasSuffix(header, ";base64") {
			return "", 0, imageInputError("AI_INPUT_INVALID", "invalid_image_data_url")
		}
		mime = strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	} else if part.Type == contract.ContentTypeImageBase64 {
		encoded = value
	} else {
		return "", 0, imageInputError("AI_INPUT_UNSUPPORTED", "reference_requires_data_url")
	}
	if !imageOneOf(mime, "", "image/png", "image/jpeg", "image/webp") {
		return "", 0, imageInputError("AI_INPUT_UNSUPPORTED", "unsupported_image_format")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(ImageReferenceMaxBytes) {
		return "", 0, imageInputError("AI_INPUT_LIMIT_EXCEEDED", "image_bytes_exceeded")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return "", 0, imageInputError("AI_INPUT_INVALID", "invalid_image_base64")
	}
	if len(data) > ImageReferenceMaxBytes {
		return "", 0, imageInputError("AI_INPUT_LIMIT_EXCEEDED", "image_bytes_exceeded")
	}
	detected := http.DetectContentType(data)
	if !imageOneOf(detected, "image/png", "image/jpeg", "image/webp") || mime != "" && mime != detected {
		return "", 0, imageInputError("AI_INPUT_INVALID", "image_mime_mismatch")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return "", 0, imageInputError("AI_INPUT_INVALID", "invalid_image")
	}
	if int64(cfg.Width)*int64(cfg.Height) > ImageReferenceMaxPixels {
		return "", 0, imageInputError("AI_INPUT_LIMIT_EXCEEDED", "image_pixels_exceeded")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return "", 0, imageInputError("AI_INPUT_INVALID", "damaged_image")
	}
	return "data:" + detected + ";base64," + base64.StdEncoding.EncodeToString(data), len(data), nil
}

func imageResponseError(code string, response *http.Response, reason string) *dto.AppError {
	return &dto.AppError{HTTPCode: 502, Code: code, Message: code, Details: map[string]any{"upstream_status": response.StatusCode, "provider_trace_id": response.Header.Get("X-Request-Id"), "reason": reason}}
}
