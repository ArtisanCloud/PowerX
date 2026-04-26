package monitorlogs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	pluginMgr "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

type PluginRuntimeTarget struct {
	PluginID string `json:"plugin_id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	State    string `json:"state"`
	APIBase  string `json:"api_base"`
}

type PluginLoggingRequest struct {
	PluginID      string
	BaseURL       string
	Authorization string
	RequestID     string
	Body          map[string]any
}

func (s *Service) ListEnabledPluginRuntimeTargets(ctx context.Context) ([]PluginRuntimeTarget, error) {
	mgr, err := getPluginManagerSafe()
	if err != nil {
		return nil, err
	}
	plugins, err := mgr.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]PluginRuntimeTarget, 0, len(plugins))
	for _, item := range plugins {
		if item.State != pluginMgr.StateEnabled {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = item.ID
		}
		items = append(items, PluginRuntimeTarget{
			PluginID: item.ID,
			Name:     name,
			Version:  item.Version,
			State:    string(item.State),
			APIBase:  fmt.Sprintf("/_p/%s/api/v1", item.ID),
		})
	}
	return items, nil
}

func (s *Service) GetPluginLoggingPolicy(ctx context.Context, req PluginLoggingRequest) (map[string]any, error) {
	return s.callPluginLoggingEndpoint(ctx, req, http.MethodGet, "/admin/runtime/logging/policy")
}

func (s *Service) UpdatePluginLoggingPolicy(ctx context.Context, req PluginLoggingRequest) (map[string]any, error) {
	return s.callPluginLoggingEndpoint(ctx, req, http.MethodPut, "/admin/runtime/logging/policy")
}

func (s *Service) ProbePluginLoggingPolicy(ctx context.Context, req PluginLoggingRequest) (map[string]any, error) {
	return s.callPluginLoggingEndpoint(ctx, req, http.MethodPost, "/admin/runtime/logging/probe")
}

func (s *Service) callPluginLoggingEndpoint(ctx context.Context, req PluginLoggingRequest, method, endpoint string) (map[string]any, error) {
	pluginID := strings.TrimSpace(req.PluginID)
	if pluginID == "" {
		return nil, fmt.Errorf("plugin_id is required")
	}
	baseURL := normalizePluginProxyBaseURL(req.BaseURL)
	targetURL := fmt.Sprintf("%s/_p/%s/api/v1%s", strings.TrimRight(baseURL, "/"), pluginID, endpoint)

	var payload io.Reader
	if req.Body != nil && method != http.MethodGet {
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal payload failed: %w", err)
		}
		payload = bytes.NewBuffer(raw)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, targetURL, payload)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if auth := strings.TrimSpace(req.Authorization); auth != "" {
		httpReq.Header.Set("Authorization", auth)
	}
	if rid := strings.TrimSpace(req.RequestID); rid != "" {
		httpReq.Header.Set("X-Request-ID", rid)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call plugin logging endpoint failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plugin logging endpoint status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	parsed := map[string]any{}
	if len(body) == 0 {
		return parsed, nil
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode plugin response failed: %w", err)
	}
	if data, ok := parsed["data"].(map[string]any); ok {
		return data, nil
	}
	return parsed, nil
}

func normalizePluginProxyBaseURL(raw string) string {
	base := strings.TrimSpace(raw)
	if base != "" {
		return strings.TrimRight(base, "/")
	}
	port := 8077
	cfg := config.GetGlobalConfig()
	if cfg != nil {
		effective := config.ResolveEffectivePorts(cfg)
		if effective.BackendPort > 0 {
			port = effective.BackendPort
		} else if cfg.Server.Port > 0 {
			port = cfg.Server.Port
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func getPluginManagerSafe() (mgr pluginMgr.Manager, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin manager unavailable: %v", r)
			mgr = nil
		}
	}()
	mgr = mgrimpl.GetPluginManager()
	if mgr == nil {
		return nil, fmt.Errorf("plugin manager unavailable")
	}
	return mgr, nil
}
