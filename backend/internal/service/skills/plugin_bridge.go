package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/datatypes"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

var (
	ErrPluginExecutorUnavailable = errors.New(ErrorCodePluginExecutorUnavailable)
)

type PluginSkillManifest struct {
	SkillID            string                 `json:"skill_id"`
	Provider           string                 `json:"provider"`
	Version            string                 `json:"version"`
	Title              string                 `json:"title,omitempty"`
	Description        string                 `json:"description"`
	IntentExamples     []string               `json:"intent_examples,omitempty"`
	ResponseGuidance   map[string][]string    `json:"response_guidance,omitempty"`
	ActionCapabilities map[string]string      `json:"action_capabilities,omitempty"`
	InputSchema        map[string]interface{} `json:"input_schema"`
	OutputSchema       map[string]interface{} `json:"output_schema,omitempty"`
	PromptRefs         []string               `json:"prompt_refs,omitempty"`
	Executor           PluginExecutorSpec     `json:"executor"`
	Visibility         string                 `json:"visibility,omitempty"`
	Status             string                 `json:"status,omitempty"`
}

type PluginExecutorSpec struct {
	Type              string            `json:"type"`
	Method            string            `json:"method,omitempty"`
	Path              string            `json:"path,omitempty"`
	Capability        string            `json:"capability"`
	PrepareCapability string            `json:"prepare_capability,omitempty"`
	ActionMap         map[string]string `json:"action_map,omitempty"`
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
		"skill_id":                    m.SkillID,
		"provider":                    m.Provider,
		"version":                     m.Version,
		"description":                 m.Description,
		"executor.capability":         m.Executor.Capability,
		"executor.prepare_capability": m.Executor.PrepareCapability,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("plugin skill manifest invalid: %s is required", field)
		}
	}
	if len(m.InputSchema) == 0 {
		return errors.New("plugin skill manifest invalid: input_schema is required")
	}
	if typ := strings.TrimSpace(m.Executor.Type); typ == "" || typ != "capability" {
		return fmt.Errorf("plugin skill manifest invalid: unsupported executor.type %q", typ)
	}
	if len(m.ActionCapabilities) == 0 && len(m.Executor.ActionMap) == 0 {
		return errors.New("plugin skill manifest invalid: action_capabilities is required")
	}
	return nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
