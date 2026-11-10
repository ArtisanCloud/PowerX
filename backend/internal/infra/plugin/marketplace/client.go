// Package marketplace 实现 Marketplace 当前公开目录与受保护下载合同。
package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Plugin struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Summary    string   `json:"summary"`
	Categories []string `json:"categories"`
	OwnerDevID string   `json:"owner_dev_id"`
}

type Target struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Version struct {
	ID       string `json:"id"`
	PluginID string `json:"plugin_id"`
	Semver   string `json:"semver"`
	Channel  string `json:"channel"`
	MinCore  string `json:"min_core"`
	Target
}

type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

type Download struct {
	DownloadID   string    `json:"download_id"`
	TrackingMode string    `json:"tracking_mode"`
	Status       string    `json:"status"`
	SignedURL    string    `json:"signed_url"`
	ExpiresAt    time.Time `json:"expires_at"`
	Target
}

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string { return e.Message }

// Client 的凭证仅用于服务端下载鉴权，不进入浏览器或对象存储请求。
type Client struct {
	mu          sync.RWMutex
	root, token string
	fingerprint string
	http        *http.Client
	deployment  deploymentProtocolState
}

func NewClient(root, token string, client *http.Client) *Client {
	return &Client{root: strings.TrimRight(strings.TrimSpace(root), "/"), token: strings.TrimSpace(token), http: client}
}
func (c *Client) Configured() bool { return c != nil && c.root != "" }
func (c *Client) DownloadReady() bool {
	if !c.Configured() {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token != ""
}

// SetInstance 只安装服务端持久化过的凭证，也用于撤销后停止继续使用旧凭证。
func (c *Client) SetInstance(token, fingerprint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token, c.fingerprint = token, fingerprint
}

// SourceID 区分不同市场，移除凭证与查询参数后才能用于来源记录和管理端展示。
func (c *Client) SourceID() string {
	if !c.Configured() {
		return ""
	}
	u, err := url.Parse(c.root)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/")
}

func (c *Client) List(ctx context.Context, os, arch string, page, size int) (Page[Plugin], error) {
	var out Page[Plugin]
	err := c.request(ctx, http.MethodGet, "/plugins", targetQuery(os, arch, page, size), nil, &out)
	if out.Items == nil {
		out.Items = []Plugin{}
	}
	return out, err
}

// Plugin 读取发布者身份；安装时必须重新请求，不能采信浏览器提交的 owner_dev_id。
func (c *Client) Plugin(ctx context.Context, id, os, arch string) (Plugin, error) {
	var out Plugin
	err := c.request(ctx, http.MethodGet, "/plugins/"+url.PathEscape(id), url.Values{"os": {os}, "arch": {arch}}, nil, &out)
	return out, err
}
func (c *Client) Versions(ctx context.Context, id, os, arch string, page, size int) (Page[Version], error) {
	var out Page[Version]
	err := c.request(ctx, http.MethodGet, "/plugins/"+url.PathEscape(id)+"/versions", targetQuery(os, arch, page, size), nil, &out)
	if out.Items == nil {
		out.Items = []Version{}
	}
	return out, err
}
func (c *Client) Download(ctx context.Context, id, version, os, arch, tenant string) (Download, error) {
	return c.DownloadWithKey(ctx, id, version, os, arch, tenant, "")
}
func (c *Client) DownloadWithKey(ctx context.Context, id, version, os, arch, tenant, key string) (Download, error) {
	var out Download
	if !c.DownloadReady() {
		return out, &Error{503, "MARKETPLACE_DOWNLOAD_AUTH_REQUIRED", "Marketplace 下载凭证未配置"}
	}
	err := c.requestWithHeaders(ctx, http.MethodPost, "/downloads", nil, map[string]string{
		"plugin_id": id, "version": version, "os": os, "arch": arch, "tenant_id": tenant,
	}, &out, "", key)
	return out, err
}
func targetQuery(os, arch string, page, size int) url.Values {
	return url.Values{"os": {os}, "arch": {arch}, "page": {fmt.Sprint(page)}, "size": {fmt.Sprint(size)}}
}
func (c *Client) request(ctx context.Context, method, suffix string, query url.Values, body, out any) error {
	return c.requestWithHeaders(ctx, method, suffix, query, body, out, "", "")
}
func (c *Client) requestWithHeaders(ctx context.Context, method, suffix string, query url.Values, body, out any, overrideToken, key string) error {
	if !c.Configured() {
		return &Error{503, "MARKETPLACE_NOT_CONFIGURED", "尚未配置 Marketplace API 地址"}
	}
	u, err := url.Parse(c.root)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return &Error{503, "MARKETPLACE_CONFIG_INVALID", "Marketplace API 地址配置无效"}
	}
	endpoint := c.root + suffix
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return &Error{503, "MARKETPLACE_CONFIG_INVALID", "Marketplace API 地址配置无效"}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.mu.RLock()
	token, fingerprint := c.token, c.fingerprint
	c.mu.RUnlock()
	if overrideToken != "" {
		token = overrideToken
	}
	if token != "" && (method == http.MethodPost || fingerprint != "") {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if fingerprint != "" {
		req.Header.Set("X-PowerX-Fingerprint", fingerprint)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	if c.http != nil {
		copy := *c.http
		client = &copy
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return &Error{502, "MARKETPLACE_UNREACHABLE", "无法连接 Marketplace，请检查服务地址和网络"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failure := &Error{502, "MARKETPLACE_UPSTREAM_ERROR", fmt.Sprintf("Marketplace 接口返回 HTTP %d", resp.StatusCode)}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			failure = &Error{502, "MARKETPLACE_AUTH_REJECTED", "Marketplace 拒绝实例或下载鉴权，请检查服务端接入状态"}
		}
		if resp.StatusCode == 404 {
			failure = &Error{404, "MARKETPLACE_NOT_FOUND", "Marketplace 中不存在对应资源"}
		}
		// 只保留兼容协商需要的受控错误码，不保存市场错误正文或消息。
		var safeCode string
		if resp.StatusCode == http.StatusBadRequest {
			var body struct {
				Error string `json:"error"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&body) == nil {
				switch body.Error {
				case "invalid_input", "instance_extension_unsupported":
					safeCode = body.Error
				}
			}
		}
		return &RequestError{Failure: failure, HTTPStatus: resp.StatusCode, UpstreamCode: safeCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return &Error{502, "MARKETPLACE_INVALID_RESPONSE", "Marketplace 响应读取失败或过大"}
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Code != 0 || len(envelope.Data) == 0 || string(envelope.Data) == "null" || (out != nil && json.Unmarshal(envelope.Data, out) != nil) {
		return &Error{502, "MARKETPLACE_INVALID_RESPONSE", "Marketplace 返回了无效响应"}
	}
	return nil
}
