package volcengine

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

func (c *videoClient) Submit(ctx context.Context, in contract.VideoRequest) (*contract.VideoTaskStatus, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	endpoint, _, err := buildSeedanceURLs(mc)
	if err != nil {
		return nil, err
	}
	content := buildSeedanceContent(in.Prompt, in.RefImages, in.RefVideos)
	for _, part := range content {
		if part["type"] == "image_url" {
			part["role"] = "first_frame"
		}
	}
	payload := map[string]any{"model": resolveModelID(mc.Model), "content": content}
	if in.Resolution != "" {
		payload["resolution"] = in.Resolution
	}
	if in.MaxDurationS > 0 {
		payload["duration"] = in.MaxDurationS
	}
	response, err := doSeedanceSubmit(ctx, endpoint, mc.APIKey, payload, mc.Timeout)
	if err != nil {
		return nil, err
	}
	id := firstNonEmptyString(response.ID, response.TaskID)
	if id == "" {
		return nil, fmt.Errorf("video provider task id missing")
	}
	return &contract.VideoTaskStatus{TaskID: id, Status: "queued", TraceID: response.RequestID}, nil
}
func (c *videoClient) Query(ctx context.Context, id string, runtime map[string]any) (*contract.VideoTaskStatus, error) {
	mc, err := modelConfigFromRuntime(runtime)
	if err != nil {
		return nil, err
	}
	submit, pattern, err := buildSeedanceURLs(mc)
	if err != nil {
		return nil, err
	}
	response, err := doSeedanceRequest(ctx, http.MethodGet, buildSeedancePollURL(submit, pattern, id), mc.APIKey, nil, mc.Timeout)
	if err != nil {
		return nil, err
	}
	result := &contract.VideoTaskStatus{TaskID: id, Status: strings.ToLower(response.Status), VideoURLs: extractSeedanceVideoURLs(response), TraceID: response.RequestID}
	if response.Error != nil {
		result.ErrorCode = fmt.Sprint(response.Error.Code)
	}
	switch result.Status {
	case "queued", "running", "succeeded", "failed", "cancelled":
	case "expired":
		result.Status = "failed"
		result.ErrorCode = "PROVIDER_TASK_EXPIRED"
	default:
		return nil, fmt.Errorf("video provider task status invalid")
	}
	if result.Status == "succeeded" && len(result.VideoURLs) == 0 {
		return nil, fmt.Errorf("video provider result missing")
	}
	return result, nil
}
func (c *videoClient) Cancel(ctx context.Context, id string, runtime map[string]any) (*contract.VideoTaskStatus, error) {
	state, err := c.Query(ctx, id, runtime)
	if err != nil {
		return nil, err
	}
	// DELETE 在完成态会删除供应商记录，只允许取消 queued，绝不把删除当取消。
	if state.Status == "cancelled" {
		state.CancelStatus = "confirmed"
		return state, nil
	}
	if state.Status != "queued" {
		state.CancelStatus = "unsupported"
		return state, nil
	}
	mc, err := modelConfigFromRuntime(runtime)
	if err != nil {
		return nil, err
	}
	submit, pattern, err := buildSeedanceURLs(mc)
	if err != nil {
		return nil, err
	}
	_, err = doSeedanceRequest(ctx, http.MethodDelete, buildSeedancePollURL(submit, pattern, id), mc.APIKey, nil, mc.Timeout)
	if err != nil {
		return nil, err
	}
	state.CancelStatus = "requested"
	return state, nil
}
