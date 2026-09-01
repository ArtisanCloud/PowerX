package hunyuan

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
)

// 腾讯云 混元（Hunyuan）LLM：TC3-HMAC-SHA256 签名 + ChatCompletions
// 参考：腾讯云通用 API 签名（TC3）与 Hunyuan ChatCompletions 接口。
type hunyuanClient struct{}

func NewLLMClient() *hunyuanClient { return &hunyuanClient{} }

const (
	defaultHunyuanEndpoint = "https://hunyuan.tencentcloudapi.com"
	hunyuanServiceName     = "hunyuan"
	hunyuanVersion         = "2023-09-01"
	hunyuanAction          = "ChatCompletions"
	hunyuanAlgorithm       = "TC3-HMAC-SHA256"
)

type hunyuanMessage struct {
	Role    string `json:"Role"`
	Content string `json:"Content"`
}

type hunyuanChatReq struct {
	Model       string           `json:"Model"`
	Messages    []hunyuanMessage `json:"Messages"`
	Temperature *float64         `json:"Temperature,omitempty"`
	TopP        *float32         `json:"TopP,omitempty"`
	MaxTokens   *int             `json:"MaxTokens,omitempty"`
	Stream      bool             `json:"Stream,omitempty"`
}

type hunyuanChatResp struct {
	Response struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
		Choices []struct {
			FinishReason string `json:"FinishReason"`
			Message      struct {
				Role    string `json:"Role"`
				Content string `json:"Content"`
			} `json:"Message"`
		} `json:"Choices"`
		Usage *struct {
			PromptTokens     int `json:"PromptTokens"`
			CompletionTokens int `json:"CompletionTokens"`
			TotalTokens      int `json:"TotalTokens"`
		} `json:"Usage,omitempty"`
		RequestId string `json:"RequestId"`
	} `json:"Response"`
}

func (c *hunyuanClient) Invoke(ctx context.Context, mc *config.ModelConfig, prompt string) (*config.InvokeResult, error) {
	if mc == nil {
		return nil, errors.New("hunyuan: missing model config")
	}
	if strings.TrimSpace(mc.SecretID) == "" || strings.TrimSpace(mc.SecretKey) == "" {
		return nil, errors.New("hunyuan: missing secret_id/secret_key")
	}

	endpoint := strings.TrimSpace(mc.Endpoint)
	if endpoint == "" {
		endpoint = defaultHunyuanEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("hunyuan: invalid base_url: %s", endpoint)
	}
	host := u.Host

	region := strings.TrimSpace(mc.Region)
	if region == "" {
		region = "ap-guangzhou"
	}

	reqBody := hunyuanChatReq{
		Model: mc.Model,
		Messages: []hunyuanMessage{
			{Role: "system", Content: mc.SystemPrompt},
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}
	if mc.Temperature > 0 {
		v := mc.Temperature
		reqBody.Temperature = &v
	}
	if mc.TopP > 0 {
		v := mc.TopP
		reqBody.TopP = &v
	}
	if mc.MaxTokens > 0 {
		v := mc.MaxTokens
		reqBody.MaxTokens = &v
	}

	action := hunyuanAction
	version := hunyuanVersion
	if mc.Extra != nil {
		if v, ok := mc.Extra["tc_action"].(string); ok && strings.TrimSpace(v) != "" {
			action = strings.TrimSpace(v)
		}
		if v, ok := mc.Extra["tc_version"].(string); ok && strings.TrimSpace(v) != "" {
			version = strings.TrimSpace(v)
		}
	}

	callOnce := func(payload []byte) (*hunyuanChatResp, []byte, error) {
		ts := time.Now().Unix()
		auth, err := tc3Authorization(
			hunyuanServiceName,
			strings.TrimSpace(mc.SecretID),
			strings.TrimSpace(mc.SecretKey),
			host,
			ts,
			payload,
		)
		if err != nil {
			return nil, nil, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
		httpReq.Header.Set("Accept", "application/json")
		httpReq.Header.Set("Host", host)
		httpReq.Header.Set("X-TC-Action", action)
		httpReq.Header.Set("X-TC-Version", version)
		httpReq.Header.Set("X-TC-Region", region)
		httpReq.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts, 10))
		httpReq.Header.Set("Authorization", auth)

		client := &http.Client{Timeout: mc.Timeout}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()

		bt, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if resp.StatusCode/100 != 2 {
			return nil, bt, fmt.Errorf("hunyuan invoke status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bt)))
		}
		var out hunyuanChatResp
		if err := json.Unmarshal(bt, &out); err != nil {
			return nil, bt, err
		}
		return &out, bt, nil
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	out, bt, err := callOnce(payload)
	if err != nil {
		return nil, err
	}
	if out.Response.Error != nil {
		// 兼容：部分账号/版本的 ChatCompletions 不接受 MaxTokens（会返回 UnknownParameter）。
		// 这里对健康检查/简单调用做一次降级重试：去掉 MaxTokens 后再请求一次。
		if reqBody.MaxTokens != nil &&
			strings.EqualFold(strings.TrimSpace(out.Response.Error.Code), "UnknownParameter") &&
			strings.Contains(out.Response.Error.Message, "MaxTokens") {
			reqBody.MaxTokens = nil
			payload2, e := json.Marshal(reqBody)
			if e != nil {
				return nil, fmt.Errorf("hunyuan error code=%s message=%s", out.Response.Error.Code, out.Response.Error.Message)
			}
			out2, _, e2 := callOnce(payload2)
			if e2 != nil {
				return nil, e2
			}
			if out2.Response.Error != nil {
				return nil, fmt.Errorf("hunyuan error code=%s message=%s", out2.Response.Error.Code, out2.Response.Error.Message)
			}
			if len(out2.Response.Choices) == 0 {
				return nil, errors.New("hunyuan: no choices")
			}
			return toInvokeResult(out2), nil
		}
		return nil, fmt.Errorf("hunyuan error code=%s message=%s", out.Response.Error.Code, out.Response.Error.Message)
	}
	_ = bt
	if len(out.Response.Choices) == 0 {
		return nil, errors.New("hunyuan: no choices")
	}
	return toInvokeResult(out), nil
}

func (c *hunyuanClient) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	return "", core.ErrStreamNotSupported
}

func toInvokeResult(resp *hunyuanChatResp) *config.InvokeResult {
	result := &config.InvokeResult{}
	if resp == nil || len(resp.Response.Choices) == 0 {
		return result
	}
	result.Text = resp.Response.Choices[0].Message.Content
	result.FinishReason = strings.TrimSpace(resp.Response.Choices[0].FinishReason)
	if resp.Response.Usage != nil {
		usage := map[string]any{}
		if resp.Response.Usage.PromptTokens > 0 {
			usage["prompt_tokens"] = resp.Response.Usage.PromptTokens
		}
		if resp.Response.Usage.CompletionTokens > 0 {
			usage["completion_tokens"] = resp.Response.Usage.CompletionTokens
		}
		if resp.Response.Usage.TotalTokens > 0 {
			usage["total_tokens"] = resp.Response.Usage.TotalTokens
		}
		if len(usage) > 0 {
			result.Usage = usage
		}
	}
	return result
}

func tc3Authorization(service, secretID, secretKey, host string, timestamp int64, payload []byte) (string, error) {
	if secretID == "" || secretKey == "" {
		return "", errors.New("tc3: missing secret")
	}
	t := time.Unix(timestamp, 0).UTC()
	date := t.Format("2006-01-02")

	hashedPayload := sha256Hex(payload)
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\n", "application/json; charset=utf-8", host)
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := strings.Join([]string{
		hunyuanAlgorithm,
		strconv.FormatInt(timestamp, 10),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := tc3SigningKey(secretKey, date, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		hunyuanAlgorithm, secretID, credentialScope, signedHeaders, signature), nil
}

func tc3SigningKey(secretKey, date, service string) []byte {
	kDate := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	kService := hmacSHA256(kDate, []byte(service))
	return hmacSHA256(kService, []byte("tc3_request"))
}

func hmacSHA256(key []byte, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
