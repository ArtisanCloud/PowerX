package contract

// ContentPart.Type 的标准枚举，避免魔法字符串
const (
	ContentTypeText        = "text"
	ContentTypeImageURL    = "image_url"
	ContentTypeImageBase64 = "image_base64"
	ContentTypeAudioURL    = "audio_url"
	ContentTypeAudioBase64 = "audio_base64"
	ContentTypeVideoURL    = "video_url"
	ContentTypeJSON        = "json"
	ContentTypeToolResult  = "tool_result"
)
