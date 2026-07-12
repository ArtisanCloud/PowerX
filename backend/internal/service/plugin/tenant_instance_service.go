package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	ErrCodeTenantPluginTenantMissing = "TENANT_PLUGIN_TENANT_MISSING"
	ErrCodeTenantPluginDisabled      = "TENANT_PLUGIN_DISABLED"
	ErrCodeTenantPluginNotFound      = "TENANT_PLUGIN_NOT_FOUND"
)

type TenantPluginInstance struct {
	TenantUUID string         `json:"tenant_uuid"`
	PluginID   string         `json:"plugin_id"`
	Version    string         `json:"version"`
	Enabled    bool           `json:"enabled"`
	Status     string         `json:"status"`
	DrainJobID string         `json:"drain_job_id,omitempty"`
	Config     map[string]any `json:"config"`
}

type TenantPluginInstanceService struct {
	repo *reposetting.PluginInstanceConfigRepository
}

func NewTenantPluginInstanceService(db *gorm.DB) *TenantPluginInstanceService {
	return NewTenantPluginInstanceServiceWithRepository(reposetting.NewPluginInstanceConfigRepository(db))
}

func NewTenantPluginInstanceServiceWithRepository(repo *reposetting.PluginInstanceConfigRepository) *TenantPluginInstanceService {
	return &TenantPluginInstanceService{repo: repo}
}

func (s *TenantPluginInstanceService) List(ctx context.Context, tenantUUID string, plugins []plugin_mgr.Plugin) ([]TenantPluginInstance, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]plugin_mgr.Plugin, len(plugins))
	ids := make([]string, 0, len(plugins))
	for _, p := range plugins {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		byID[id] = p
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []TenantPluginInstance{}, nil
	}
	bindings, err := s.requireRepo().ListTenantPluginBindings(ctx, reposetting.ListTenantPluginOptions{
		TenantUUIDs: []string{tenantUUID},
		PluginIDs:   ids,
		Key:         reposetting.KeyClientCredentials,
		OnlyEnabled: false,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TenantPluginInstance, 0, len(bindings))
	for _, binding := range bindings {
		if strings.TrimSpace(binding.TenantUUID) != tenantUUID {
			continue
		}
		p, ok := byID[strings.TrimSpace(binding.PluginID)]
		if !ok {
			continue
		}
		cfg, err := s.requireRepo().Get(ctx, tenantUUID, p.ID, reposetting.KeyClientCredentials)
		if err != nil {
			return nil, err
		}
		if cfg == nil {
			continue
		}
		out = append(out, toTenantPluginInstance(tenantUUID, p, cfg))
	}
	return out, nil
}

func (s *TenantPluginInstanceService) Get(ctx context.Context, tenantUUID, pluginID string, p plugin_mgr.Plugin) (*TenantPluginInstance, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodeTenantPluginNotFound, "缺少插件ID", errors.New("plugin_id required"))
	}
	cfg, err := s.requireRepo().Get(ctx, tenantUUID, pluginID, reposetting.KeyClientCredentials)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	instance := toTenantPluginInstance(tenantUUID, p, cfg)
	return &instance, nil
}

func (s *TenantPluginInstanceService) Enable(ctx context.Context, tenantUUID string, p plugin_mgr.Plugin, config map[string]any) (*TenantPluginInstance, string, string, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, "", "", err
	}
	pluginID := strings.TrimSpace(p.ID)
	if pluginID == "" {
		return nil, "", "", dto.NewErrorWithCode(http.StatusBadRequest, ErrCodeTenantPluginNotFound, "缺少插件ID", errors.New("plugin_id required"))
	}
	if err := s.EnsurePluginAcceptsNewUsage(ctx, pluginID); err != nil {
		return nil, "", "", err
	}

	clientID, clientSecret, err := s.ensureCredentials(ctx, tenantUUID, pluginID)
	if err != nil {
		return nil, "", "", err
	}
	cfg, err := s.requireRepo().Get(ctx, tenantUUID, pluginID, reposetting.KeyClientCredentials)
	if err != nil {
		return nil, "", "", err
	}
	if cfg == nil {
		return nil, "", "", dto.NewErrorWithCode(http.StatusInternalServerError, ErrCodeTenantPluginNotFound, "租户插件实例创建失败", errors.New("tenant plugin instance missing after credentials ensure"))
	}
	cfg.ValueJSON = datatypes.JSON(mergePluginInstanceConfig(cfg.ValueJSON, clientID, config))
	cfg.Enabled = true
	cfg.Status = dbsetting.PluginInstanceStatusEnabled
	cfg.DrainJobID = ""
	cfg.DrainRequestedAt = nil
	cfg.DrainedAt = nil

	if err := s.requireRepo().Upsert(ctx, cfg); err != nil {
		return nil, "", "", err
	}
	instance := toTenantPluginInstance(tenantUUID, p, cfg)
	return &instance, clientID, clientSecret, nil
}

func (s *TenantPluginInstanceService) Disable(ctx context.Context, tenantUUID, pluginID string) (*TenantPluginInstance, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	pluginID = strings.TrimSpace(pluginID)
	cfg, err := s.requireRepo().Get(ctx, tenantUUID, pluginID, reposetting.KeyClientCredentials)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, dto.NewErrorWithCode(http.StatusNotFound, ErrCodeTenantPluginNotFound, "租户插件实例不存在", errors.New("tenant plugin instance not found"))
	}
	previousStatus := dbsetting.NormalizePluginInstanceStatus(cfg.Status, cfg.Enabled)
	cfg.Enabled = false
	switch previousStatus {
	case dbsetting.PluginInstanceStatusDrainingRequested, dbsetting.PluginInstanceStatusDisabledByPlatform:
		now := time.Now().UTC()
		cfg.Status = dbsetting.PluginInstanceStatusDrained
		cfg.DrainedAt = &now
	default:
		cfg.Status = dbsetting.PluginInstanceStatusDisabled
	}
	if err := s.requireRepo().Upsert(ctx, cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.DrainJobID) != "" {
		drainSvc := NewPluginDrainJobServiceFromRepository(s.requireRepo())
		if _, err := drainSvc.RefreshDrainJobProgress(ctx, cfg.DrainJobID); err != nil {
			return nil, err
		}
	}
	instance := toTenantPluginInstance(tenantUUID, plugin_mgr.Plugin{ID: pluginID}, cfg)
	return &instance, nil
}

func (s *TenantPluginInstanceService) IsEnabled(ctx context.Context, tenantUUID, pluginID string) (bool, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return false, err
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return false, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodeTenantPluginNotFound, "缺少插件ID", errors.New("plugin_id required"))
	}
	cfg, err := s.requireRepo().Get(ctx, tenantUUID, pluginID, reposetting.KeyClientCredentials)
	if err != nil {
		return false, err
	}
	return cfg != nil && cfg.Enabled, nil
}

func (s *TenantPluginInstanceService) EnsurePluginAcceptsNewUsage(ctx context.Context, pluginID string) error {
	drainSvc := NewPluginDrainJobServiceFromRepository(s.requireRepo())
	return drainSvc.EnsurePluginAcceptsNewUsage(ctx, pluginID)
}

func (s *TenantPluginInstanceService) RequireEnabled(ctx context.Context, tenantUUID, pluginID string) error {
	enabled, err := s.IsEnabled(ctx, tenantUUID, pluginID)
	if err != nil {
		return err
	}
	if !enabled {
		return dto.NewErrorWithCode(http.StatusForbidden, ErrCodeTenantPluginDisabled, "当前租户未启用该插件", errors.New("tenant plugin instance disabled"))
	}
	return nil
}

func (s *TenantPluginInstanceService) CountByPlugin(ctx context.Context, pluginID string) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodeTenantPluginNotFound, "缺少插件ID", errors.New("plugin_id required"))
	}
	return s.requireRepo().CountTenantPluginBindings(ctx, reposetting.ListTenantPluginOptions{
		PluginIDs:   []string{pluginID},
		Key:         reposetting.KeyClientCredentials,
		OnlyEnabled: false,
	})
}

func (s *TenantPluginInstanceService) CountActiveByPlugin(ctx context.Context, pluginID string) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodeTenantPluginNotFound, "缺少插件ID", errors.New("plugin_id required"))
	}
	return s.requireRepo().CountActiveTenantPluginBindings(ctx, pluginID)
}

func (s *TenantPluginInstanceService) DisableByTenant(ctx context.Context, tenantUUID string) (int64, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return 0, err
	}
	return s.requireRepo().DisableTenantPluginBindings(ctx, tenantUUID, reposetting.KeyClientCredentials)
}

func (s *TenantPluginInstanceService) requireRepo() *reposetting.PluginInstanceConfigRepository {
	if s == nil || s.repo == nil {
		panic("tenant plugin instance service requires repository")
	}
	return s.repo
}

func (s *TenantPluginInstanceService) ensureCredentials(ctx context.Context, tenantUUID, pluginID string) (string, string, error) {
	cfg, err := s.requireRepo().Get(ctx, tenantUUID, pluginID, reposetting.KeyClientCredentials)
	if err != nil {
		return "", "", err
	}
	if cfg != nil && len(cfg.ValueJSON) > 0 {
		return extractClientID(cfg.ValueJSON), "", nil
	}
	clientID := fmt.Sprintf("%s.%s", pluginID, tenantUUID)
	clientSecret := utils.RandomString(48)
	hash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	now := time.Now().Unix()
	raw, _ := json.Marshal(map[string]any{
		"client_id":          clientID,
		"client_secret_hash": string(hash),
		"secret_version":     1,
		"issued_at":          now,
	})
	if err := s.requireRepo().Upsert(ctx, &dbsetting.PluginInstanceConfig{
		TenantUUID: tenantUUID,
		PluginID:   pluginID,
		Key:        reposetting.KeyClientCredentials,
		ValueJSON:  datatypes.JSON(raw),
		Enabled:    true,
	}); err != nil {
		return "", "", err
	}
	return clientID, clientSecret, nil
}

func canonicalTenantUUID(raw string) (string, error) {
	canonical, err := reqctx.CanonicalTenantUUID(strings.TrimSpace(raw))
	if err != nil || canonical == "" {
		if err == nil {
			err = errors.New("tenant_uuid required")
		}
		return "", dto.NewErrorWithCode(http.StatusUnauthorized, ErrCodeTenantPluginTenantMissing, "缺少租户上下文", err)
	}
	return canonical, nil
}

func toTenantPluginInstance(tenantUUID string, p plugin_mgr.Plugin, cfg *dbsetting.PluginInstanceConfig) TenantPluginInstance {
	version := strings.TrimSpace(p.Version)
	if version == "" {
		version = extractVersion(cfg.ValueJSON)
	}
	return TenantPluginInstance{
		TenantUUID: tenantUUID,
		PluginID:   strings.TrimSpace(cfg.PluginID),
		Version:    version,
		Enabled:    cfg.Enabled,
		Status:     dbsetting.NormalizePluginInstanceStatus(cfg.Status, cfg.Enabled),
		DrainJobID: strings.TrimSpace(cfg.DrainJobID),
		Config:     extractPublicConfig(cfg.ValueJSON),
	}
}

func extractPublicConfig(raw datatypes.JSON) map[string]any {
	var wrapper struct {
		Config map[string]any `json:"config"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &wrapper) != nil || wrapper.Config == nil {
		return map[string]any{}
	}
	return wrapper.Config
}

func extractClientID(raw datatypes.JSON) string {
	var wrapper struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal(raw, &wrapper)
	return strings.TrimSpace(wrapper.ClientID)
}

func extractVersion(raw datatypes.JSON) string {
	var wrapper struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &wrapper)
	return strings.TrimSpace(wrapper.Version)
}

func mergePluginInstanceConfig(raw datatypes.JSON, clientID string, config map[string]any) []byte {
	var doc map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &doc)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	if strings.TrimSpace(clientID) != "" {
		doc["client_id"] = strings.TrimSpace(clientID)
	}
	doc["config"] = normalizeConfig(config)
	out, _ := json.Marshal(doc)
	return out
}

func normalizeConfig(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}
