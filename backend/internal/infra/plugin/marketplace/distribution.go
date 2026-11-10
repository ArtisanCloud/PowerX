package marketplace

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

// RequestError 保留上游状态供补报区分可重试故障与凭证/合同错误，不保留上游正文。
type RequestError struct {
	Failure      *Error
	HTTPStatus   int
	UpstreamCode string
}

func (e *RequestError) Error() string { return e.Failure.Error() }
func (e *RequestError) Unwrap() error { return e.Failure }

func Retryable(err error) bool {
	var request *RequestError
	if errors.As(err, &request) {
		return request.HTTPStatus == 429 || request.HTTPStatus >= 500
	}
	var failure *Error
	return errors.As(err, &failure) && failure.Status == 502
}
func AuthenticationRejected(err error) bool {
	var request *RequestError
	return errors.As(err, &request) && (request.HTTPStatus == 401 || request.HTTPStatus == 403)
}

type Registration struct {
	Instance struct {
		ID string `json:"id"`
	} `json:"instance"`
	InstanceToken string `json:"instance_token"`
	Protocol      string `json:"protocol"`
	DeploymentResponse
}

const AutomaticRegistrationProtocol = "powerx.instance-registration/v1"

type Discovery struct {
	Service          string   `json:"service"`
	Protocol         string   `json:"protocol"`
	Features         []string `json:"features,omitempty"`
	HeartbeatSeconds int      `json:"heartbeat_seconds,omitempty"`
}

func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	var result Discovery
	public := NewClient(c.root, "", c.http)
	err := public.request(ctx, http.MethodGet, "/host/instances/discovery", nil, nil, &result)
	if err == nil && (result.Service != "powerx-marketplace" || result.Protocol != AutomaticRegistrationProtocol) {
		err = &Error{502, "MARKETPLACE_REGISTRATION_UNSUPPORTED", "Marketplace 尚未支持自动实例登记"}
	}
	return result, err
}

// RegisterAutomatic 使用宿主先行持久化的随机凭证，响应丢失后仍可安全重试。
func (c *Client) RegisterAutomatic(ctx context.Context, credential, fingerprint, version, name string) (Registration, error) {
	var result Registration
	err := c.requestWithHeaders(ctx, http.MethodPost, "/host/instances/register", nil, map[string]string{
		"fingerprint_sha256": fingerprint, "powerx_version": strings.TrimPrefix(version, "v"), "os": runtime.GOOS, "arch": runtime.GOARCH, "name": name,
	}, &result, credential, "")
	if err == nil && result.Protocol != AutomaticRegistrationProtocol {
		err = &Error{502, "MARKETPLACE_REGISTRATION_UNSUPPORTED", "Marketplace 自动登记响应无效"}
	}
	return result, err
}

type TransferReport struct {
	EventID    string     `json:"event_id"`
	Status     string     `json:"status"`
	SHA256     string     `json:"sha256,omitempty"`
	Size       *int64     `json:"size,omitempty"`
	Stage      string     `json:"stage,omitempty"`
	ErrorCode  string     `json:"error_code,omitempty"`
	Message    string     `json:"message,omitempty"`
	ReportedAt *time.Time `json:"reported_at,omitempty"`
}

func (c *Client) Register(ctx context.Context, enrollment, fingerprint, version string) (Registration, error) {
	var result Registration
	err := c.requestWithHeaders(ctx, http.MethodPost, "/host/instances/register", nil, map[string]string{
		"fingerprint_sha256": fingerprint, "powerx_version": strings.TrimPrefix(version, "v"), "os": runtime.GOOS, "arch": runtime.GOARCH,
	}, &result, enrollment, "")
	return result, err
}
func (c *Client) Heartbeat(ctx context.Context, version string) error {
	return c.request(ctx, http.MethodPost, "/host/instances/heartbeat", nil, map[string]string{"powerx_version": strings.TrimPrefix(version, "v")}, nil)
}
func (c *Client) Report(ctx context.Context, downloadID string, event TransferReport) error {
	if downloadID == "" || strings.ContainsAny(downloadID, "/?#") {
		return &Error{400, "MARKETPLACE_DOWNLOAD_ID_INVALID", "下载记录 ID 无效"}
	}
	return c.request(ctx, http.MethodPost, "/downloads/"+url.PathEscape(downloadID)+"/events", nil, event, nil)
}
