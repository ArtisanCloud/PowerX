package agnes

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

type videoClient struct{}

// NewVideoClient 构造使用统一 TaskBus 生命周期的 Agnes 视频客户端。
func NewVideoClient() *videoClient                   { return &videoClient{} }
func (*videoClient) Cap() contract.ModelCapabilities { return contract.ModelCapabilities{} }
func (*videoClient) Health(context.Context) error    { return nil }

// Validate 在创建 Core 任务前校验模式和引用的组合，避免无效请求进入供应商队列。
func (*videoClient) Validate(in contract.VideoRequest) error {
	mc, err := modelConfig(in.Runtime)
	if err != nil {
		return err
	}
	if !oneOf(mc.Model, "agnes-video-2.5", "agnes-video-2.5-flash") {
		return parameterError("model_key", "unsupported_model")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return parameterError("inputs", "prompt_required")
	}
	seconds := in.MaxDurationS
	if seconds == 0 {
		seconds = 5
	}
	if seconds < 4 || seconds > 12 {
		return parameterError("max_duration_sec", "out_of_range")
	}
	resolution := strings.ToUpper(in.Resolution)
	if resolution == "" {
		resolution = "720P"
	}
	if !oneOf(resolution, "720P", "1080P", "2K") || mc.Model == "agnes-video-2.5-flash" && resolution != "720P" {
		return parameterError("resolution", "invalid_enum")
	}
	if in.FPS != 0 {
		return parameterError("fps", "unsupported_parameter")
	}
	ratio := setting(in.Runtime, mc, "aspect_ratio", "16:9")
	if !oneOf(ratio, "21:9", "16:9", "4:3", "1:1", "3:4", "9:16") {
		return parameterError("aspect_ratio", "invalid_enum")
	}
	maxImages := 8
	if mc.Model == "agnes-video-2.5-flash" {
		maxImages = 5
	}
	if len(in.RefImages) > maxImages || len(in.RefVideos) > 0 {
		return parameterError("inputs", "unsupported_reference_count")
	}
	for _, image := range in.RefImages {
		u, e := url.Parse(image.URL)
		if image.Type != contract.ContentTypeImageURL || e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return parameterError("inputs", "public_image_url_required")
		}
	}
	mode := setting(in.Runtime, mc, "mode", "text")
	switch mode {
	case "text":
		if len(in.RefImages) != 0 {
			return parameterError("mode", "text_disallows_images")
		}
	case "reference":
		if len(in.RefImages) == 0 {
			return parameterError("mode", "reference_requires_images")
		}
	case "keyframe":
		if len(in.RefImages) < 1 || len(in.RefImages) > 2 {
			return parameterError("mode", "keyframe_requires_one_or_two_images")
		}
	default:
		return parameterError("mode", "invalid_enum")
	}
	return nil
}

func (c *videoClient) Generate(ctx context.Context, in contract.VideoRequest) (*contract.VideoResponse, error) {
	state, err := c.Submit(ctx, in)
	if err != nil {
		return nil, err
	}
	mc, _ := modelConfig(in.Runtime)
	return &contract.VideoResponse{Provider: "agnes", Model: mc.Model, TaskID: state.TaskID, TraceID: state.TraceID}, nil
}

func (c *videoClient) Submit(ctx context.Context, in contract.VideoRequest) (*contract.VideoTaskStatus, error) {
	if err := c.Validate(in); err != nil {
		return nil, err
	}
	mc, _ := modelConfig(in.Runtime)
	endpoint, err := apiURL(mc, "/v1/videos")
	if err != nil {
		return nil, err
	}
	seconds := in.MaxDurationS
	if seconds == 0 {
		seconds = 5
	}
	size := strings.ToUpper(in.Resolution)
	if size == "" {
		size = "720P"
	}
	mode := setting(in.Runtime, mc, "mode", "text")
	payload := map[string]any{"model": mc.Model, "prompt": in.Prompt, "seconds": strconv.Itoa(seconds), "size": size, "mode": mode, "aspect_ratio": setting(in.Runtime, mc, "aspect_ratio", "16:9"), "n": 1}
	if mode == "reference" {
		images := []string{}
		for _, image := range in.RefImages {
			images = append(images, image.URL)
		}
		payload["images"] = images
	} else if mode == "keyframe" {
		payload["first_frame"] = in.RefImages[0].URL
		if len(in.RefImages) == 2 {
			payload["last_frame"] = in.RefImages[1].URL
		}
	}
	var out struct {
		VideoID   string `json:"video_id"`
		RequestID string `json:"request_id"`
	}
	headers, err := call(ctx, mc, http.MethodPost, endpoint, payload, &out, 1<<20)
	if err != nil {
		return nil, err
	}
	// id/task_id 不能代替 video_id；后者才是官方查询句柄。
	if !validVideoID(out.VideoID) {
		return nil, outputError()
	}
	trace := out.RequestID
	if trace == "" {
		trace = headers.Get("X-Request-ID")
	}
	return &contract.VideoTaskStatus{TaskID: out.VideoID, Status: "queued", TraceID: trace}, nil
}

func validVideoID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 256 && !strings.ContainsAny(id, "/?#\\\r\n")
}

func (*videoClient) Query(ctx context.Context, id string, runtime map[string]any) (*contract.VideoTaskStatus, error) {
	if !validVideoID(id) {
		return nil, fmt.Errorf("agnes video id invalid")
	}
	mc, err := modelConfig(runtime)
	if err != nil {
		return nil, err
	}
	if !oneOf(mc.Model, "agnes-video-2.5", "agnes-video-2.5-flash") {
		return nil, parameterError("model_key", "unsupported_model")
	}
	endpoint, err := apiURL(mc, "/agnesapi")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("video_id", id)
	query.Set("model_name", mc.Model)
	endpoint.RawQuery = query.Encode()
	var out struct {
		Status    string `json:"status"`
		URL       string `json:"url"`
		RequestID string `json:"request_id"`
		Error     *struct {
			Code any `json:"code"`
		} `json:"error"`
	}
	headers, err := call(ctx, mc, http.MethodGet, endpoint, nil, &out, 1<<20)
	if err != nil {
		return nil, err
	}
	state := &contract.VideoTaskStatus{TaskID: id, TraceID: out.RequestID}
	if state.TraceID == "" {
		state.TraceID = headers.Get("X-Request-ID")
	}
	switch strings.ToLower(out.Status) {
	case "queued", "pending":
		state.Status = "queued"
	case "running", "in_progress", "processing":
		state.Status = "running"
	case "completed":
		if strings.TrimSpace(out.URL) == "" {
			return nil, outputError()
		}
		state.Status = "succeeded"
		state.VideoURLs = []string{out.URL}
	case "failed":
		state.Status = "failed"
		state.ErrorCode = "AGNES_VIDEO_FAILED"
		if out.Error != nil && out.Error.Code != nil {
			state.ErrorCode = fmt.Sprint(out.Error.Code)
		}
	case "cancelled", "canceled":
		state.Status = "cancelled"
	default:
		return nil, outputError()
	}
	return state, nil
}

func (c *videoClient) Cancel(ctx context.Context, id string, runtime map[string]any) (*contract.VideoTaskStatus, error) {
	// 官方尚未发布取消协议；只读查询，不猜测 DELETE，也不把删除冒充取消。
	state, err := c.Query(ctx, id, runtime)
	if err != nil {
		return nil, err
	}
	state.CancelStatus = "unsupported"
	if state.Status == "cancelled" {
		state.CancelStatus = "confirmed"
	}
	return state, nil
}
