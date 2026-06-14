package skills

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/datatypes"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

var (
	ErrPluginNotInstalled        = errors.New(ErrorCodePluginNotInstalled)
	ErrPluginExecutorUnavailable = errors.New(ErrorCodePluginExecutorUnavailable)
	ErrPluginContextMissing      = errors.New(ErrorCodePluginContextMissing)
	ErrPluginCapabilityMismatch  = errors.New(ErrorCodePluginCapabilityMismatch)
)

type PluginEndpointResolver interface {
	ResolvePluginEndpoint(ctx context.Context, providerPluginID string) (string, error)
}

type StaticPluginEndpointResolver map[string]string

func (r StaticPluginEndpointResolver) ResolvePluginEndpoint(_ context.Context, providerPluginID string) (string, error) {
	endpoint := strings.TrimSpace(r[providerPluginID])
	if endpoint == "" {
		return "", fmt.Errorf("%w: %s", ErrPluginNotInstalled, providerPluginID)
	}
	return endpoint, nil
}

type PluginSkillManifest struct {
	SkillID        string                 `json:"skill_id"`
	Provider       string                 `json:"provider"`
	Version        string                 `json:"version"`
	Title          string                 `json:"title,omitempty"`
	Description    string                 `json:"description"`
	IntentExamples []string               `json:"intent_examples,omitempty"`
	InputSchema    map[string]interface{} `json:"input_schema"`
	OutputSchema   map[string]interface{} `json:"output_schema,omitempty"`
	PromptRefs     []string               `json:"prompt_refs,omitempty"`
	Executor       PluginExecutorSpec     `json:"executor"`
	Visibility     string                 `json:"visibility,omitempty"`
	Status         string                 `json:"status,omitempty"`
}

type PluginExecutorSpec struct {
	Type       string `json:"type"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Capability string `json:"capability"`
}

type PluginSkillDiscoveryService struct {
	importSvc *ImportService
	http      *http.Client
}

func NewPluginSkillDiscoveryService(importSvc *ImportService) *PluginSkillDiscoveryService {
	if importSvc == nil {
		panic("plugin skill discovery service requires import service")
	}
	return &PluginSkillDiscoveryService{
		importSvc: importSvc,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *PluginSkillDiscoveryService) WithHTTPClient(client *http.Client) *PluginSkillDiscoveryService {
	if s == nil {
		return nil
	}
	if client != nil {
		s.http = client
	}
	return s
}

type DiscoverPluginSkillsInput struct {
	ProviderPluginID string
	BaseURL          string
	BearerToken      string
	Operator         string
	BundleURI        string
}

func (s *PluginSkillDiscoveryService) DiscoverAndImport(ctx context.Context, in DiscoverPluginSkillsInput) ([]*skillmodel.SkillRegistryRecord, error) {
	provider := strings.TrimSpace(in.ProviderPluginID)
	baseURL := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if provider == "" {
		return nil, errors.New("provider_plugin_id is required")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("%w: plugin base_url is required", ErrPluginExecutorUnavailable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/plugin/skills", nil)
	if err != nil {
		return nil, err
	}
	if token := strings.TrimSpace(in.BearerToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPluginExecutorUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: discovery returned %s", ErrPluginExecutorUnavailable, resp.Status)
	}
	var envelope struct {
		Items []PluginSkillManifest `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	out := make([]*skillmodel.SkillRegistryRecord, 0, len(envelope.Items))
	for _, manifest := range envelope.Items {
		if err := ValidatePluginSkillManifest(manifest); err != nil {
			return nil, err
		}
		if manifest.Provider != provider {
			return nil, fmt.Errorf("manifest provider %q does not match plugin %q", manifest.Provider, provider)
		}
		raw, err := json.Marshal(manifest)
		if err != nil {
			return nil, err
		}
		bundleURI := strings.TrimSpace(in.BundleURI)
		if bundleURI == "" {
			bundleURI = "plugin://" + provider + "/" + manifest.SkillID + "/" + manifest.Version
		}
		saved, err := s.importSvc.ImportDraft(ctx, ImportRequest{
			SkillID:    manifest.SkillID,
			Version:    manifest.Version,
			Source:     skillmodel.SkillSourcePlugin,
			BundleURI:  bundleURI,
			Checksum:   "sha256:" + sha256Hex(raw),
			SourceURL:  baseURL,
			SourceRef:  provider,
			Manifest:   datatypes.JSON(raw),
			Operator:   in.Operator,
			ImportType: ImportTypeUpload,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, saved)
	}
	return out, nil
}

func ValidatePluginSkillManifest(m PluginSkillManifest) error {
	required := map[string]string{
		"skill_id":            m.SkillID,
		"provider":            m.Provider,
		"version":             m.Version,
		"description":         m.Description,
		"executor.path":       m.Executor.Path,
		"executor.capability": m.Executor.Capability,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("plugin skill manifest invalid: %s is required", field)
		}
	}
	if len(m.InputSchema) == 0 {
		return errors.New("plugin skill manifest invalid: input_schema is required")
	}
	if typ := strings.TrimSpace(m.Executor.Type); typ != "" && typ != "plugin_http" {
		return fmt.Errorf("plugin skill manifest invalid: unsupported executor.type %q", typ)
	}
	method := strings.ToUpper(strings.TrimSpace(m.Executor.Method))
	if method != "" && method != http.MethodPost {
		return fmt.Errorf("plugin skill manifest invalid: executor.method must be POST")
	}
	return nil
}

type PluginSkillHTTPExecutor struct {
	resolver PluginEndpointResolver
	http     *http.Client
	tokenFn  func(context.Context, string) (string, error)
}

func NewPluginSkillHTTPExecutor(resolver PluginEndpointResolver) *PluginSkillHTTPExecutor {
	if resolver == nil {
		panic("plugin skill http executor requires endpoint resolver")
	}
	return &PluginSkillHTTPExecutor{
		resolver: resolver,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *PluginSkillHTTPExecutor) WithHTTPClient(client *http.Client) *PluginSkillHTTPExecutor {
	if e == nil {
		return nil
	}
	if client != nil {
		e.http = client
	}
	return e
}

func (e *PluginSkillHTTPExecutor) WithDelegatedTokenProvider(fn func(context.Context, string) (string, error)) *PluginSkillHTTPExecutor {
	if e == nil {
		return nil
	}
	e.tokenFn = fn
	return e
}

func (e *PluginSkillHTTPExecutor) CanHandle(in ExecuteInput) bool {
	if strings.TrimSpace(in.Source) != skillmodel.SkillSourcePlugin {
		return false
	}
	executor := manifestExecutor(in.Manifest)
	return strings.TrimSpace(executor.Type) == "" || strings.EqualFold(strings.TrimSpace(executor.Type), "plugin_http")
}

func (e *PluginSkillHTTPExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]interface{}, error) {
	provider := strings.TrimSpace(manifestString(in.Manifest, "provider"))
	executor := manifestExecutor(in.Manifest)
	if provider == "" {
		return nil, fmt.Errorf("%w: provider is required", ErrPluginNotInstalled)
	}
	if strings.TrimSpace(executor.Path) == "" {
		return nil, fmt.Errorf("%w: executor.path is required", ErrPluginExecutorUnavailable)
	}
	if strings.TrimSpace(executor.Capability) == "" {
		return nil, fmt.Errorf("%w: executor.capability is required", ErrPluginCapabilityMismatch)
	}
	if in.CapabilityID != "" && !strings.EqualFold(in.CapabilityID, executor.Capability) {
		return nil, fmt.Errorf("%w: request capability %s does not match %s", ErrPluginCapabilityMismatch, in.CapabilityID, executor.Capability)
	}
	pluginCtx, err := buildPluginInvocationContext(in, provider, executor.Capability)
	if err != nil {
		return nil, err
	}
	baseURL, err := e.resolver.ResolvePluginEndpoint(ctx, provider)
	if err != nil {
		return nil, err
	}
	endpoint, err := joinURL(baseURL, "/api/v1/plugin/skills/invoke")
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"skill_id": in.SkillID,
		"version":  in.Version,
		"input":    in.Payload,
		"context":  pluginCtx,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.tokenFn == nil {
		return nil, fmt.Errorf("%w: delegated token provider is required", ErrPluginExecutorUnavailable)
	}
	token, err := e.tokenFn(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPluginExecutorUnavailable, err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: delegated bearer is empty", ErrPluginExecutorUnavailable)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPluginExecutorUnavailable, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, mapPluginHTTPError(resp.StatusCode, respBody)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func buildPluginInvocationContext(in ExecuteInput, provider, capability string) (map[string]interface{}, error) {
	ctx := map[string]interface{}{}
	for k, v := range in.Context {
		ctx[k] = v
	}
	ctx["tenant_uuid"] = firstPluginValue(in.TenantUUID, ctx["tenant_uuid"])
	ctx["skill_id"] = firstPluginValue(in.SkillID, ctx["skill_id"])
	ctx["trace_id"] = firstPluginValue(in.TraceID, ctx["trace_id"])
	ctx["provider_plugin_id"] = provider
	ctx["plugin_id"] = provider
	ctx["capability"] = firstPluginValue(capability, ctx["capability"], ctx["capability_id"])
	ctx["capability_id"] = ctx["capability"]
	required := []string{"tenant_uuid", "user_uuid", "agent_id", "session_id", "skill_id", "trace_id"}
	for _, field := range required {
		if firstPluginValue(ctx[field]) == "" {
			return nil, fmt.Errorf("%w: %s is required", ErrPluginContextMissing, field)
		}
	}
	return ctx, nil
}

func firstPluginValue(values ...interface{}) string {
	for _, value := range values {
		if s := strings.TrimSpace(fmt.Sprint(value)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func manifestExecutor(manifest map[string]interface{}) PluginExecutorSpec {
	raw, ok := manifest["executor"].(map[string]interface{})
	if !ok {
		return PluginExecutorSpec{}
	}
	return PluginExecutorSpec{
		Type:       strings.TrimSpace(fmt.Sprint(raw["type"])),
		Method:     strings.TrimSpace(fmt.Sprint(raw["method"])),
		Path:       strings.TrimSpace(fmt.Sprint(raw["path"])),
		Capability: strings.TrimSpace(fmt.Sprint(raw["capability"])),
	}
}

func manifestString(manifest map[string]interface{}, key string) string {
	return strings.TrimSpace(fmt.Sprint(manifest[key]))
}

func joinURL(base, path string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return u.String(), nil
}

func mapPluginHTTPError(status int, body []byte) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	msg := strings.TrimSpace(envelope.Error.Message)
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	switch envelope.Error.Code {
	case ErrorCodePluginContextMissing:
		return fmt.Errorf("%w: %s", ErrPluginContextMissing, msg)
	case ErrorCodePluginCapabilityMismatch:
		return fmt.Errorf("%w: %s", ErrPluginCapabilityMismatch, msg)
	case ErrorCodePluginExecutorUnavailable:
		return fmt.Errorf("%w: %s", ErrPluginExecutorUnavailable, msg)
	}
	if status == http.StatusBadRequest {
		return fmt.Errorf("%w: %s", ErrPluginContextMissing, msg)
	}
	return fmt.Errorf("%w: plugin executor returned HTTP %d: %s", ErrPluginExecutorUnavailable, status, msg)
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
