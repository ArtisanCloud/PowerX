package volcengine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const defaultVolcengineImageEndpoint = "https://visual.volcengineapi.com"

const (
	defaultVolcengineAction     = "CVSync2AsyncSubmitTask"
	defaultVolcenginePollAction = "CVSync2AsyncGetResult"
	defaultVolcengineVersion    = "2022-08-31"
	defaultVolcengineService    = "cv"
)

type imageClient struct {
	rawProvider string
}

func NewImageClient(rawProvider string) *imageClient {
	return &imageClient{rawProvider: strings.TrimSpace(rawProvider)}
}

func (c *imageClient) Cap() contract.ModelCapabilities {
	return contract.ModelCapabilities{}
}

func (c *imageClient) Health(ctx context.Context) error {
	return nil
}

type volcengineSubmitReq struct {
	ReqKey      string   `json:"req_key"`
	Prompt      string   `json:"prompt,omitempty"`
	ImageURLs   []string `json:"image_urls,omitempty"`
	Size        int      `json:"size,omitempty"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
	Scale       float64  `json:"scale,omitempty"`
	ForceSingle bool     `json:"force_single,omitempty"`
	MinRatio    float64  `json:"min_ratio,omitempty"`
	MaxRatio    float64  `json:"max_ratio,omitempty"`
}

type volcengineSubmitResp struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      *struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type volcengineResultReq struct {
	ReqKey  string `json:"req_key"`
	TaskID  string `json:"task_id"`
	ReqJSON string `json:"req_json,omitempty"`
}

type volcengineResultData struct {
	Status           string   `json:"status"`
	ImageURLs        []string `json:"image_urls"`
	BinaryDataBase64 []string `json:"binary_data_base64"`
}

type volcengineResultResp struct {
	Code      int                   `json:"code"`
	Message   string                `json:"message"`
	RequestID string                `json:"request_id"`
	Data      *volcengineResultData `json:"data"`
}

func (c *imageClient) Generate(ctx context.Context, in contract.ImageRequest) (*contract.ImageResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, errors.New("volcengine image: missing model config")
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, errors.New("volcengine image: prompt required")
	}
	if strings.TrimSpace(mc.SecretID) == "" || strings.TrimSpace(mc.SecretKey) == "" {
		return nil, errors.New("volcengine image: missing secret_id/secret_key")
	}

	modelID := resolveModelID(mc.Model)
	if modelID == "" {
		return nil, errors.New("volcengine image: missing req_key")
	}
	imageURLs, err := refImagesToURLs(in.RefImages)
	if err != nil {
		return nil, err
	}

	submitReq := volcengineSubmitReq{
		ReqKey:    modelID,
		Prompt:    prompt,
		ImageURLs: imageURLs,
	}
	applySizeParams(&submitReq, in.Size)
	applyRuntimeParams(&submitReq, in.Runtime)
	applyExtraParams(&submitReq, mc.Extra)

	submitBody, err := json.Marshal(submitReq)
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
	submitResp, err := doVolcengineRequest[volcengineSubmitResp](ctx, submitEndpoint, mc, submitBody)
	if err != nil {
		return nil, err
	}
	if submitResp.Code != 10000 || submitResp.Data == nil || strings.TrimSpace(submitResp.Data.TaskID) == "" {
		return nil, fmt.Errorf("volcengine image submit failed code=%d message=%s request_id=%s", submitResp.Code, submitResp.Message, submitResp.RequestID)
	}

	pollReq := volcengineResultReq{
		ReqKey:  modelID,
		TaskID:  submitResp.Data.TaskID,
		ReqJSON: buildResultReqJSON(in.Runtime, mc.Extra),
	}
	pollBody, err := json.Marshal(pollReq)
	if err != nil {
		return nil, err
	}

	images, urls, traceID, err := pollVolcengineResult(ctx, pollEndpoint, mc, pollBody)
	if err != nil {
		return nil, err
	}

	return &contract.ImageResponse{
		Images:    images,
		ImageURLs: urls,
		Provider:  strings.TrimSpace(c.rawProvider),
		Model:     strings.TrimSpace(mc.Model),
		LatencyMS: int(time.Since(start).Milliseconds()),
		TraceID:   traceID,
	}, nil
}

func modelConfigFromRuntime(runtime map[string]any) (*config.ModelConfig, error) {
	if runtime == nil {
		return nil, errors.New("image runtime config missing")
	}
	if raw, ok := runtime["config"]; ok {
		switch v := raw.(type) {
		case *config.ModelConfig:
			return v, nil
		case config.ModelConfig:
			return &v, nil
		}
	}
	if raw, ok := runtime["model_config"]; ok {
		switch v := raw.(type) {
		case *config.ModelConfig:
			return v, nil
		case config.ModelConfig:
			return &v, nil
		}
	}
	return nil, errors.New("image runtime config invalid")
}

func buildVolcengineEndpoint(mc *config.ModelConfig, action string) (string, error) {
	base := strings.TrimRight(mc.Endpoint, "/")
	if base == "" {
		base = defaultVolcengineImageEndpoint
	}
	version := resolveVersion(mc)
	q := url.Values{}
	q.Set("Action", action)
	q.Set("Version", version)
	return base + "/?" + q.Encode(), nil
}

func resolveAction(mc *config.ModelConfig) string {
	if mc != nil && mc.Extra != nil {
		if v, ok := mc.Extra["action"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return defaultVolcengineAction
}

func resolvePollAction(mc *config.ModelConfig) string {
	if mc != nil && mc.Extra != nil {
		if v, ok := mc.Extra["action_poll"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return defaultVolcenginePollAction
}

func resolveVersion(mc *config.ModelConfig) string {
	if mc != nil && mc.Extra != nil {
		if v, ok := mc.Extra["version"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return defaultVolcengineVersion
}

func resolveRegion(mc *config.ModelConfig) string {
	region := "cn-north-1"
	if mc != nil && strings.TrimSpace(mc.Region) != "" {
		region = strings.TrimSpace(mc.Region)
	}
	return region
}

func resolveService(mc *config.ModelConfig) string {
	if mc != nil && mc.Extra != nil {
		if v, ok := mc.Extra["service"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return defaultVolcengineService
}

func resolveModelID(model string) string {
	raw := strings.TrimSpace(model)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		return strings.TrimSpace(parts[1])
	}
	return raw
}

func refImagesToURLs(parts []contract.ContentPart) ([]string, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case contract.ContentTypeImageURL:
			if strings.TrimSpace(part.URL) != "" {
				urls = append(urls, strings.TrimSpace(part.URL))
			}
		case contract.ContentTypeImageBase64:
			return nil, errors.New("volcengine image: base64 reference images not supported")
		}
	}
	return urls, nil
}

func applySizeParams(req *volcengineSubmitReq, size string) {
	if req == nil {
		return
	}
	w, h := parseSize(size)
	if w > 0 && h > 0 {
		req.Width = w
		req.Height = h
	}
}

func applyRuntimeParams(req *volcengineSubmitReq, runtime map[string]any) {
	if req == nil || runtime == nil {
		return
	}
	if v, ok := runtime["scale"]; ok {
		if f, ok2 := toFloat64(v); ok2 {
			req.Scale = f
		}
	}
	if v, ok := runtime["force_single"]; ok {
		if b, ok2 := v.(bool); ok2 {
			req.ForceSingle = b
		}
	}
	if v, ok := runtime["min_ratio"]; ok {
		if f, ok2 := toFloat64(v); ok2 {
			req.MinRatio = f
		}
	}
	if v, ok := runtime["max_ratio"]; ok {
		if f, ok2 := toFloat64(v); ok2 {
			req.MaxRatio = f
		}
	}
}

func applyExtraParams(req *volcengineSubmitReq, extra map[string]any) {
	if req == nil || extra == nil {
		return
	}
	if v, ok := extra["scale"]; ok {
		if f, ok2 := toFloat64(v); ok2 {
			req.Scale = f
		}
	}
	if v, ok := extra["force_single"]; ok {
		if b, ok2 := v.(bool); ok2 {
			req.ForceSingle = b
		}
	}
	if v, ok := extra["min_ratio"]; ok {
		if f, ok2 := toFloat64(v); ok2 {
			req.MinRatio = f
		}
	}
	if v, ok := extra["max_ratio"]; ok {
		if f, ok2 := toFloat64(v); ok2 {
			req.MaxRatio = f
		}
	}
}

func buildResultReqJSON(runtime map[string]any, extra map[string]any) string {
	payload := map[string]any{"return_url": true}
	if extra != nil {
		if v, ok := extra["result_req_json"]; ok {
			switch vv := v.(type) {
			case string:
				if strings.TrimSpace(vv) != "" {
					return strings.TrimSpace(vv)
				}
			case map[string]any:
				for k, val := range vv {
					payload[k] = val
				}
			}
		}
	}
	if runtime != nil {
		if v, ok := runtime["result_req_json"]; ok {
			switch vv := v.(type) {
			case string:
				if strings.TrimSpace(vv) != "" {
					return strings.TrimSpace(vv)
				}
			case map[string]any:
				for k, val := range vv {
					payload[k] = val
				}
			}
		}
	}
	if len(payload) == 0 {
		return ""
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

func pollVolcengineResult(ctx context.Context, endpoint string, mc *config.ModelConfig, body []byte) ([][]byte, []string, string, error) {
	interval := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return nil, nil, "", ctx.Err()
		case <-time.After(interval):
		}
		resp, err := doVolcengineRequest[volcengineResultResp](ctx, endpoint, mc, body)
		if err != nil {
			return nil, nil, "", err
		}
		if resp.Code != 10000 {
			return nil, nil, resp.RequestID, fmt.Errorf("volcengine image result failed code=%d message=%s request_id=%s", resp.Code, resp.Message, resp.RequestID)
		}
		if resp.Data == nil {
			return nil, nil, resp.RequestID, errors.New("volcengine image result missing data")
		}
		switch strings.ToLower(strings.TrimSpace(resp.Data.Status)) {
		case "done":
			images, urls, err := extractResultImages(resp.Data)
			return images, urls, resp.RequestID, err
		case "in_queue", "generating":
			continue
		case "not_found", "expired":
			return nil, nil, resp.RequestID, fmt.Errorf("volcengine image result status=%s", resp.Data.Status)
		default:
			return nil, nil, resp.RequestID, fmt.Errorf("volcengine image result status=%s", resp.Data.Status)
		}
	}
}

func extractResultImages(data *volcengineResultData) ([][]byte, []string, error) {
	if data == nil {
		return nil, nil, errors.New("volcengine image: empty result")
	}
	if len(data.ImageURLs) > 0 {
		return nil, data.ImageURLs, nil
	}
	if len(data.BinaryDataBase64) == 0 {
		return nil, nil, errors.New("volcengine image: empty result")
	}
	imgs := make([][]byte, 0, len(data.BinaryDataBase64))
	for _, item := range data.BinaryDataBase64 {
		decoded, err := base64.StdEncoding.DecodeString(item)
		if err != nil {
			return nil, nil, fmt.Errorf("volcengine image: decode base64 failed: %w", err)
		}
		imgs = append(imgs, decoded)
	}
	return imgs, nil, nil
}

func doVolcengineRequest[T any](ctx context.Context, endpoint string, mc *config.ModelConfig, body []byte) (*T, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := signVolcengineRequest(httpReq, mc.SecretID, mc.SecretKey, resolveRegion(mc), resolveService(mc), body); err != nil {
		return nil, err
	}

	start := time.Now()
	httpClient := volcengineHTTPClient(mc)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	logger.InfoF(ctx, "[ai-image] volcengine http_call provider=%s model=%s status=%d latency_ms=%d", strings.TrimSpace(mc.Provider), strings.TrimSpace(mc.Model), resp.StatusCode, time.Since(start).Milliseconds())

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("volcengine image status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func parseSize(size string) (int, int) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return w, h
}

func toFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func volcengineHTTPClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		to = 5 * time.Minute
	}
	return &http.Client{Timeout: to}
}

func signVolcengineRequest(req *http.Request, accessKey, secretKey, region, service string, body []byte) error {
	if req == nil {
		return errors.New("volcengine image: request nil")
	}
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return errors.New("volcengine image: missing access key")
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(body)

	req.Host = req.URL.Host
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Date", amzDate)
	req.Header.Set("X-Content-Sha256", payloadHash)

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalQueryString(req.URL.Query())
	canonicalHeaders := buildCanonicalHeaders(req)
	signedHeaders := "content-type;host;x-content-sha256;x-date"

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		amzDate,
		dateStamp + "/" + region + "/" + service + "/request",
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	authorization := fmt.Sprintf(
		"HMAC-SHA256 Credential=%s/%s/%s/%s/request, SignedHeaders=%s, Signature=%s",
		strings.TrimSpace(accessKey),
		dateStamp,
		region,
		service,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authorization)
	return nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func deriveSigningKey(secretKey, date, region, service string) []byte {
	kDate := hmacSHA256([]byte(secretKey), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0)
	for _, key := range keys {
		vals := values[key]
		sort.Strings(vals)
		for _, val := range vals {
			pairs = append(pairs, encodeQuery(key)+"="+encodeQuery(val))
		}
	}
	return strings.Join(pairs, "&")
}

func encodeQuery(input string) string {
	encoded := url.QueryEscape(input)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func buildCanonicalHeaders(req *http.Request) string {
	contentType := strings.TrimSpace(req.Header.Get("Content-Type"))
	host := strings.TrimSpace(req.Header.Get("Host"))
	sha := strings.TrimSpace(req.Header.Get("X-Content-Sha256"))
	xDate := strings.TrimSpace(req.Header.Get("X-Date"))

	return strings.Join([]string{
		"content-type:" + contentType,
		"host:" + host,
		"x-content-sha256:" + sha,
		"x-date:" + xDate,
		"",
	}, "\n")
}
