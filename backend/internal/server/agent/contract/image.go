package contract

import "context"

// Image 生成/编辑 客户端
type ImageClient interface {
	Generate(ctx context.Context, in ImageRequest) (*ImageResponse, error)
	// 未来若支持编辑/变体，可扩展：Edit(ctx, in ImageEditRequest) (*ImageResponse, error)

	Cap() ModelCapabilities
	Health(ctx context.Context) error
}

type ImageRequest struct {
	Prompt    string
	RefImages []ContentPart // 可选：图生图（image_url / image_base64）

	// 常见参数
	Size    string // e.g. "1024x1024"
	Quality string // "standard" | "hd"
	Format  string // "png" | "jpeg" | "webp"

	// 运行时覆盖（路由/驱动自由取用）
	Runtime map[string]any
}

type ImageResponse struct {
	// 二选一或并存（由驱动决定返回哪种）
	Images    [][]byte // 直接二进制
	ImageURLs []string

	Provider string
	Model    string

	// 可选：用量/时延/追踪
	Usage     map[string]int // e.g. {"input_tokens":0, "output_tokens":0}
	LatencyMS int
	TraceID   string
}
