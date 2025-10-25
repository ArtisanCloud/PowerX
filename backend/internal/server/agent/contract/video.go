package contract

import "context"

// 文生视频 / 图生视频（多数供应商为异步任务，这里做成“同步拿结果或任务句柄”的最小集合）
type VideoClient interface {
	Generate(ctx context.Context, in VideoRequest) (*VideoResponse, error)

	Cap() ModelCapabilities
	Health(ctx context.Context) error
}

type VideoRequest struct {
	Prompt       string
	RefImages    []ContentPart // 参考帧（image_url/base64）
	RefVideos    []ContentPart // 参考视频（video_url）
	Resolution   string        // "720p" | "1080p" | "4k"
	FPS          int
	MaxDurationS int

	Runtime map[string]any
}

type VideoResponse struct {
	// 供应商若异步，可能只返回任务信息；驱动可选择把最终产物转存后回传URL
	Videos    [][]byte // 直接二进制
	VideoURLs []string // 或 URL
	TaskID    string   // 若为异步任务，这里返回任务ID
	PollURL   string   // 可选：轮询地址（若供应商要求）

	Provider string
	Model    string

	Usage     map[string]int
	LatencyMS int
	TraceID   string
}
