package contract

import "context"

// 文本转语音（TTS）
type TTSClient interface {
	Synthesize(ctx context.Context, in TTSRequest) (*TTSResponse, error)

	Cap() ModelCapabilities
	Health(ctx context.Context) error
}

type TTSRequest struct {
	Text    string
	Voice   string  // e.g. "alloy", "female_1", "cn_male"
	Speed   float64 // 1.0 为正常
	Format  string  // "mp3" | "wav" | "pcm"
	Runtime map[string]any
}

type TTSResponse struct {
	Audio    []byte // 直接二进制
	AudioURL string // 或者 URL（两种可并存，至少一种）
	Provider string
	Model    string

	Usage     map[string]int
	LatencyMS int
	TraceID   string
}

// 语音转文本（ASR）
type ASRClient interface {
	Transcribe(ctx context.Context, in ASRRequest) (*ASRResponse, error)

	Cap() ModelCapabilities
	Health(ctx context.Context) error
}

type ASRRequest struct {
	Audio   ContentPart // audio_url / audio_base64
	Lang    string      // 目标语言(可选)；空则自动识别
	Runtime map[string]any
}

type ASRResponse struct {
	Text     string
	Provider string
	Model    string

	Usage     map[string]int
	LatencyMS int
	TraceID   string
}
