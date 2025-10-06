// services/agent/contract/types.go
package contract

import flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

type Modality string

const (
	ModLLM      Modality = "llm"
	ModImage    Modality = "image"
	ModEmbed    Modality = "embedding"
	ModAudioTTS Modality = "audio_tts"
	ModAudioASR Modality = "audio_asr"
	ModVideo    Modality = "video"
	ModRerank   Modality = "rerank"
)

type ToolSpec = flowschema.ToolSpec
type JSONSchema = flowschema.JSONSchema

// 能力特征（用于路由与UI配置提示）
type ModelCapabilities struct {
	SupportsStream, SupportsTools, SupportsVision bool
	SupportsAudioIn, SupportsAudioOut             bool
	SupportsJSONMode                              bool
	MaxInputTokens, MaxOutputTokens               int
	DefaultDims                                   int // embedding
}

const HeaderKeySecWebSocketProtocol = "Sec-WebSocket-Protocol"
