package volcengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
)

const (
	defaultSeedanceSubmitPath = "/api/v1/contents/generations/tasks"
	defaultSeedancePollPath   = "/api/v1/contents/generations/tasks/%s"
)

type videoClient struct {
	rawProvider string
}

func NewVideoClient(rawProvider string) *videoClient {
	return &videoClient{rawProvider: strings.TrimSpace(rawProvider)}
}

func (c *videoClient) Cap() contract.ModelCapabilities {
	return contract.ModelCapabilities{}
}

func (c *videoClient) Health(ctx context.Context) error {
	return nil
}

type volcengineVideoSubmitResp struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      *struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type volcengineVideoResultResp struct {
	Code      int                    `json:"code"`
	Message   string                 `json:"message"`
	RequestID string                 `json:"request_id"`
	Data      map[string]interface{} `json:"data"`
}

type seedanceTaskResp struct {
	ID      string                 `json:"id"`
	TaskID  string                 `json:"task_id"`
	Status  string                 `json:"status"`
	State   string                 `json:"state"`
	Content map[string]interface{} `json:"content"`
	Output  map[string]interface{} `json:"output"`
	Data    map[string]interface{} `json:"data"`
	Error   *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
}

func (c *videoClient) Generate(ctx context.Context, in contract.VideoRequest) (*contract.VideoResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, errors.New("volcengine video: missing model config")
	}
	modelID := resolveModelID(mc.Model)
	if modelID == "" {
		return nil, errors.New("volcengine video: missing model")
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, errors.New("volcengine video: prompt required")
	}

	if shouldUseSeedanceAPI(modelID, mc.Extra) {
		return c.generateBySeedanceTasks(ctx, in, mc, modelID)
	}
	return c.generateByVisualAPI(ctx, in, mc, modelID)
}

func (c *videoClient) generateByVisualAPI(ctx context.Context, in contract.VideoRequest, mc *config.ModelConfig, modelID string) (*contract.VideoResponse, error) {
	if strings.TrimSpace(mc.SecretID) == "" || strings.TrimSpace(mc.SecretKey) == "" {
		return nil, errors.New("volcengine video: missing secret_id/secret_key")
	}

	submitPayload := map[string]interface{}{
		"req_key": modelID,
		"prompt":  strings.TrimSpace(in.Prompt),
	}
	refImages, err := refImagesToURLs(in.RefImages)
	if err != nil {
		return nil, err
	}
	if len(refImages) > 0 {
		submitPayload["image_urls"] = refImages
	}
	refVideos := refVideosToURLs(in.RefVideos)
	if len(refVideos) > 0 {
		submitPayload["video_urls"] = refVideos
	}
	if w, h := parseSize(in.Resolution); w > 0 && h > 0 {
		submitPayload["width"] = w
		submitPayload["height"] = h
	}
	if in.FPS > 0 {
		submitPayload["fps"] = in.FPS
	}
	if in.MaxDurationS > 0 {
		submitPayload["duration"] = in.MaxDurationS
	}
	mergePayload(submitPayload, mc.Extra)

	submitBody, err := json.Marshal(submitPayload)
	if err != nil {
		return nil, err
	}

	submitEndpoint, err := buildVolcengineEndpoint(mc, resolveAction(mc))
	if err != nil {
		return nil, err
	}
	pollEndpoint, err := buildVolcengineEndpoint(mc, resolvePollAction(mc))
	if err != nil {
		return nil, err
	}

	start := time.Now()
	submitResp, err := doVolcengineRequest[volcengineVideoSubmitResp](ctx, submitEndpoint, mc, submitBody)
	if err != nil {
		return nil, err
	}
	if submitResp.Code != 10000 || submitResp.Data == nil || strings.TrimSpace(submitResp.Data.TaskID) == "" {
		return nil, fmt.Errorf("volcengine video submit failed code=%d message=%s request_id=%s", submitResp.Code, submitResp.Message, submitResp.RequestID)
	}

	pollPayload := map[string]interface{}{
		"req_key":  modelID,
		"task_id":  strings.TrimSpace(submitResp.Data.TaskID),
		"req_json": buildResultReqJSON(in.Runtime, mc.Extra),
	}
	pollBody, err := json.Marshal(pollPayload)
	if err != nil {
		return nil, err
	}

	urls, traceID, err := pollVolcengineVideoResult(ctx, pollEndpoint, mc, pollBody)
	if err != nil {
		return nil, err
	}

	return &contract.VideoResponse{
		VideoURLs: urls,
		TaskID:    strings.TrimSpace(submitResp.Data.TaskID),
		Provider:  strings.TrimSpace(c.rawProvider),
		Model:     strings.TrimSpace(mc.Model),
		LatencyMS: int(time.Since(start).Milliseconds()),
		TraceID:   traceID,
	}, nil
}

func (c *videoClient) generateBySeedanceTasks(ctx context.Context, in contract.VideoRequest, mc *config.ModelConfig, modelID string) (*contract.VideoResponse, error) {
	if strings.TrimSpace(mc.APIKey) == "" {
		return nil, errors.New("volcengine seedance video: missing api_key")
	}

	submitURL, pollURLPattern, err := buildSeedanceURLs(mc)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"model":   modelID,
		"content": buildSeedanceContent(strings.TrimSpace(in.Prompt), in.RefImages, in.RefVideos),
		"prompt":  strings.TrimSpace(in.Prompt),
	}
	params := map[string]any{}
	if in.Resolution != "" {
		params["resolution"] = strings.TrimSpace(in.Resolution)
	}
	if in.FPS > 0 {
		params["fps"] = in.FPS
	}
	if in.MaxDurationS > 0 {
		params["duration"] = in.MaxDurationS
	}
	if len(params) > 0 {
		payload["parameters"] = params
	}
	if mc.Extra != nil {
		if extParams, ok := mc.Extra["parameters"].(map[string]any); ok {
			if payloadParams, ok2 := payload["parameters"].(map[string]any); ok2 {
				for k, v := range extParams {
					if _, exists := payloadParams[k]; !exists {
						payloadParams[k] = v
					}
				}
			} else {
				payload["parameters"] = extParams
			}
		}
	}

	start := time.Now()
	submitResp, err := doSeedanceSubmit(ctx, submitURL, strings.TrimSpace(mc.APIKey), payload, mc.Timeout)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(firstNonEmptyString(
		submitResp.TaskID,
		submitResp.ID,
		getString(submitResp.Data, "task_id"),
		getString(submitResp.Data, "id"),
	))
	if taskID == "" {
		return nil, errors.New("volcengine seedance video: empty task id")
	}

	pollURL := buildSeedancePollURL(submitURL, pollURLPattern, taskID)
	urls, traceID, err := pollSeedanceResult(ctx, pollURL, strings.TrimSpace(mc.APIKey), mc.Timeout)
	if err != nil {
		return nil, err
	}

	return &contract.VideoResponse{
		VideoURLs: urls,
		TaskID:    taskID,
		PollURL:   pollURL,
		Provider:  strings.TrimSpace(c.rawProvider),
		Model:     strings.TrimSpace(mc.Model),
		LatencyMS: int(time.Since(start).Milliseconds()),
		TraceID:   traceID,
	}, nil
}

func shouldUseSeedanceAPI(modelID string, extra map[string]any) bool {
	mid := strings.ToLower(strings.TrimSpace(modelID))
	if strings.Contains(mid, "seedance") {
		return true
	}
	if extra == nil {
		return false
	}
	if v, ok := extra["api_path_submit"].(string); ok && strings.TrimSpace(v) != "" {
		return true
	}
	if v, ok := extra["api_path_poll"].(string); ok && strings.TrimSpace(v) != "" {
		return true
	}
	if v, ok := extra["base_url"].(string); ok && strings.Contains(strings.ToLower(strings.TrimSpace(v)), "volces.com") {
		return true
	}
	return false
}

func buildSeedanceURLs(mc *config.ModelConfig) (submitURL string, pollURLPattern string, err error) {
	base := strings.TrimSpace(mc.Endpoint)
	if mc.Extra != nil {
		if v, ok := mc.Extra["base_url"].(string); ok && strings.TrimSpace(v) != "" {
			base = strings.TrimSpace(v)
		}
	}
	if base == "" || strings.Contains(base, "{your-id}") {
		return "", "", errors.New("volcengine seedance video: invalid base_url, please set your operator endpoint")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", "", fmt.Errorf("volcengine seedance video: invalid base_url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", errors.New("volcengine seedance video: invalid base_url")
	}

	submitPath := defaultSeedanceSubmitPath
	pollPath := defaultSeedancePollPath
	if mc.Extra != nil {
		if v, ok := mc.Extra["api_path_submit"].(string); ok && strings.TrimSpace(v) != "" {
			submitPath = strings.TrimSpace(v)
		}
		if v, ok := mc.Extra["api_path_poll"].(string); ok && strings.TrimSpace(v) != "" {
			pollPath = strings.TrimSpace(v)
		}
	}
	if !strings.HasPrefix(submitPath, "/") {
		submitPath = "/" + submitPath
	}
	if !strings.HasPrefix(pollPath, "/") {
		pollPath = "/" + pollPath
	}

	u.Path = strings.TrimRight(path.Join("/", submitPath), "/")
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), pollPath, nil
}

func buildSeedancePollURL(submitURL, pollPattern, taskID string) string {
	if strings.TrimSpace(taskID) == "" {
		return ""
	}
	if strings.Contains(pollPattern, "%s") {
		if u, err := url.Parse(submitURL); err == nil {
			u.Path = fmt.Sprintf(strings.TrimSpace(pollPattern), url.PathEscape(strings.TrimSpace(taskID)))
			u.RawPath = ""
			return u.String()
		}
	}
	return strings.TrimRight(submitURL, "/") + "/" + url.PathEscape(strings.TrimSpace(taskID))
}

func doSeedanceSubmit(ctx context.Context, submitURL, apiKey string, payload map[string]any, timeout time.Duration) (*seedanceTaskResp, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := doSeedanceRequest(ctx, http.MethodPost, submitURL, apiKey, raw, timeout)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func pollSeedanceResult(ctx context.Context, pollURL, apiKey string, timeout time.Duration) ([]string, string, error) {
	interval := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(interval):
		}

		resp, err := doSeedanceRequest(ctx, http.MethodGet, pollURL, apiKey, nil, timeout)
		if err != nil {
			return nil, "", err
		}

		status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(resp.Status, resp.State, getString(resp.Data, "status"))))
		switch status {
		case "", "pending", "queued", "running", "processing", "in_progress", "in_queue", "generating":
			continue
		case "done", "completed", "succeeded", "success":
			urls := extractSeedanceVideoURLs(resp)
			if len(urls) == 0 {
				return nil, "", errors.New("volcengine seedance video: empty result")
			}
			traceID := strings.TrimSpace(firstNonEmptyString(getString(resp.Data, "request_id"), getString(resp.Output, "request_id")))
			return urls, traceID, nil
		case "failed", "error", "cancelled", "canceled", "expired":
			errMsg := "volcengine seedance video failed"
			if resp.Error != nil && strings.TrimSpace(resp.Error.Message) != "" {
				errMsg = strings.TrimSpace(resp.Error.Message)
			}
			return nil, "", errors.New(errMsg)
		default:
			return nil, "", fmt.Errorf("volcengine seedance video result status=%s", status)
		}
	}
}

func doSeedanceRequest(ctx context.Context, method, endpoint, apiKey string, body []byte, timeout time.Duration) (*seedanceTaskResp, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeoutOrDefault(timeout)}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("volcengine seedance video status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out seedanceTaskResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("volcengine seedance video decode failed: %w", err)
	}
	return &out, nil
}

func timeoutOrDefault(to time.Duration) time.Duration {
	if to <= 0 {
		return 5 * time.Minute
	}
	return to
}

func buildSeedanceContent(prompt string, refImages []contract.ContentPart, refVideos []contract.ContentPart) []map[string]any {
	content := make([]map[string]any, 0, 1+len(refImages)+len(refVideos))
	if strings.TrimSpace(prompt) != "" {
		content = append(content, map[string]any{
			"type": "text",
			"text": strings.TrimSpace(prompt),
		})
	}
	for _, p := range refImages {
		if p.Type != contract.ContentTypeImageURL || strings.TrimSpace(p.URL) == "" {
			continue
		}
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": strings.TrimSpace(p.URL)},
		})
	}
	for _, p := range refVideos {
		if p.Type != contract.ContentTypeVideoURL || strings.TrimSpace(p.URL) == "" {
			continue
		}
		content = append(content, map[string]any{
			"type":      "video_url",
			"video_url": map[string]any{"url": strings.TrimSpace(p.URL)},
		})
	}
	return content
}

func extractSeedanceVideoURLs(resp *seedanceTaskResp) []string {
	if resp == nil {
		return nil
	}
	candidates := []any{resp.Content, resp.Output, resp.Data, resp}
	for _, c := range candidates {
		if out := extractVideoURLsFromAny(c); len(out) > 0 {
			return out
		}
	}
	return nil
}

func extractVideoURLsFromAny(raw any) []string {
	switch v := raw.(type) {
	case map[string]any:
		for _, key := range []string{"video_urls", "videos", "video_url", "output_url", "url"} {
			if out := readStringArray(v[key]); len(out) > 0 {
				return out
			}
		}
		for _, item := range v {
			if out := extractVideoURLsFromAny(item); len(out) > 0 {
				return out
			}
		}
	case []any:
		collected := make([]string, 0)
		for _, item := range v {
			if out := extractVideoURLsFromAny(item); len(out) > 0 {
				collected = append(collected, out...)
			}
		}
		if len(collected) > 0 {
			return collected
		}
	}
	return nil
}

func refVideosToURLs(parts []contract.ContentPart) []string {
	if len(parts) == 0 {
		return nil
	}
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != contract.ContentTypeVideoURL {
			continue
		}
		if strings.TrimSpace(part.URL) == "" {
			continue
		}
		urls = append(urls, strings.TrimSpace(part.URL))
	}
	return urls
}

func mergePayload(dst map[string]interface{}, extra map[string]interface{}) {
	if dst == nil || extra == nil {
		return
	}
	for k, v := range extra {
		if _, ok := dst[k]; ok {
			continue
		}
		dst[k] = v
	}
}

func pollVolcengineVideoResult(ctx context.Context, endpoint string, mc *config.ModelConfig, body []byte) ([]string, string, error) {
	interval := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(interval):
		}

		resp, err := doVolcengineRequest[volcengineVideoResultResp](ctx, endpoint, mc, body)
		if err != nil {
			return nil, "", err
		}
		if resp.Code != 10000 {
			return nil, resp.RequestID, fmt.Errorf("volcengine video result failed code=%d message=%s request_id=%s", resp.Code, resp.Message, resp.RequestID)
		}
		status := strings.ToLower(strings.TrimSpace(getString(resp.Data, "status")))
		switch status {
		case "done":
			urls := extractVideoURLs(resp.Data)
			if len(urls) == 0 {
				return nil, resp.RequestID, errors.New("volcengine video: empty result")
			}
			return urls, resp.RequestID, nil
		case "in_queue", "generating", "running", "":
			continue
		default:
			return nil, resp.RequestID, fmt.Errorf("volcengine video result status=%s", status)
		}
	}
}

func extractVideoURLs(data map[string]interface{}) []string {
	if data == nil {
		return nil
	}
	keys := []string{"video_urls", "videos", "video_url", "output_url", "url"}
	for _, key := range keys {
		if vals := readStringArray(data[key]); len(vals) > 0 {
			return vals
		}
	}
	return nil
}

func getString(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	v, _ := data[key]
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func readStringArray(raw interface{}) []string {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if strings.TrimSpace(item) == "" {
				continue
			}
			out = append(out, strings.TrimSpace(item))
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyString(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}
