package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides helpers for diagnostics endpoints.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Options configures the client.
type Options struct {
	Token      string
	HTTPClient *http.Client
}

// NewClient constructs a diagnostics client.
func NewClient(baseURL string, opts Options) *Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   opts.Token,
		http:    httpClient,
	}
}

// CreateReportRequest mirrors admin API payload.
type CreateReportRequest struct {
	TenantUUID  string            `json:"tenant_uuid"`
	PluginID    string            `json:"pluginId"`
	TraceID     string            `json:"traceId"`
	Notes       string            `json:"notes"`
	LogPointers []string          `json:"logPointers"`
	Summary     map[string]any    `json:"summary"`
	Metadata    map[string]string `json:"metadata"`
	Severity    string            `json:"severity"`
}

// CreateReportResponse wraps newly created report.
type CreateReportResponse struct {
	Data any `json:"data"`
}

// ExportLogsResponse mirrors export payload.
type ExportLogsResponse struct {
	Data struct {
		ReportID string `json:"reportId"`
		URL      string `json:"url"`
	} `json:"data"`
}

// CreateReport triggers diagnostics report generation.
func (c *Client) CreateReport(ctx context.Context, req CreateReportRequest) (*CreateReportResponse, error) {
	return c.post(ctx, "/internal/plugins/debug/report", req)
}

// ExportLogs fetches log bundle location.
func (c *Client) ExportLogs(ctx context.Context, payload map[string]string) (*ExportLogsResponse, error) {
	return c.postExport(ctx, "/internal/plugins/debug/logs/export", payload)
}

func (c *Client) post(ctx context.Context, path string, payload any) (*CreateReportResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("debug client %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out CreateReportResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) postExport(ctx context.Context, path string, payload any) (*ExportLogsResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("debug client %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out ExportLogsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
