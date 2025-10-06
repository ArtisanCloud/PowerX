package setting

// internal/service/setting/plugin_instance_config_service.go

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
)

const (
	// 统一把插件凭证落在同一个 key 下
	KeyClientCredentials = "auth.credentials"
)

// 插件凭证（落在 value_json）
type ClientCredential struct {
	ClientID          string   `json:"client_id"`
	ClientSecretHash  string   `json:"client_secret_hash"`
	SecretVersion     int      `json:"secret_version,omitempty"` // 可选：便于灰度/双活切换
	AllowedAudiences  []string `json:"allowed_audiences,omitempty"`
	AllowedScopes     []string `json:"allowed_scopes,omitempty"`
	AllowedActorKinds []string `json:"allowed_actor_kinds,omitempty"` // e.g. USER/CUSTOMER/SUPPLIER...
	ExpiresAt         *int64   `json:"expires_at,omitempty"`          // unix 秒，可选
	IssuedAt          int64    `json:"issued_at"`                     // unix 秒
	RotatedAt         *int64   `json:"rotated_at,omitempty"`
}

// Service
type PluginInstanceConfigService struct {
    PluginInstanceRepo *reposetting.PluginInstanceConfigRepository
}

func NewPluginInstanceConfigService(deps *shared.Deps) *PluginInstanceConfigService {
	return &PluginInstanceConfigService{
		PluginInstanceRepo: reposetting.NewPluginInstanceConfigRepository(deps.DB),
	}
}

/* ==================== 基本读写 ==================== */

func (s *PluginInstanceConfigService) Get(ctx context.Context, tenantID uint64, pluginID, key string) (*dbsetting.PluginInstanceConfig, error) {
	return s.PluginInstanceRepo.Get(ctx, tenantID, strings.TrimSpace(pluginID), strings.TrimSpace(key))
}

func (s *PluginInstanceConfigService) Upsert(ctx context.Context, m *dbsetting.PluginInstanceConfig) error {
	return s.PluginInstanceRepo.Upsert(ctx, m)
}

func (s *PluginInstanceConfigService) SetEnabled(ctx context.Context, tenantID uint64, pluginID string, enabled bool) error {
	return s.PluginInstanceRepo.SetEnabled(ctx, tenantID, strings.TrimSpace(pluginID), enabled)
}

func (s *PluginInstanceConfigService) ListByTenantAndPlugin(ctx context.Context, tenantID uint64, pluginID string) ([]*dbsetting.PluginInstanceConfig, error) {
	return s.PluginInstanceRepo.ListByTenantAndPlugin(ctx, tenantID, strings.TrimSpace(pluginID))
}

func (s *PluginInstanceConfigService) ListEnabledPluginsByTenant(ctx context.Context, tenantID uint64) ([]string, error) {
	return s.PluginInstanceRepo.ListEnabledPluginsByTenant(ctx, tenantID)
}

/* ============== client_id / client_secret 生命周期 ============== */

// EnsureCredentials：若不存在则创建并返回“明文 secret”（只此一次）；若已存在仅返回 client_id，secret 置空
func (s *PluginInstanceConfigService) EnsureCredentials(
	ctx context.Context,
	tenantID uint64,
	pluginID string,
	opts *ClientCredential, // 可传入能力约束（aud/scopes/actorKinds），为 nil 则用默认
) (clientID, clientSecretPlain string, err error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return "", "", errors.New("plugin_id required")
	}

	cfg, err := s.PluginInstanceRepo.Get(ctx, tenantID, pluginID, KeyClientCredentials)
	if err != nil {
		return "", "", err
	}

	// 已有：返回 client_id，但不再返回明文 secret
	if cfg != nil && len(cfg.ValueJSON) > 0 {
		var cc ClientCredential
		_ = json.Unmarshal(cfg.ValueJSON, &cc)
		return cc.ClientID, "", nil
	}

	// 生成：client_id 建议稳定可读；secret 仅展示一次
	clientID = fmt.Sprintf("%s.%d", pluginID, tenantID) // 也可用 uuid/slug
	clientSecretPlain = utils.RandomString(48)
	hash, err := bcrypt.GenerateFromPassword([]byte(clientSecretPlain), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}

	now := time.Now().Unix()
	cc := &ClientCredential{
		ClientID:         clientID,
		ClientSecretHash: string(hash),
		SecretVersion:    1,
		IssuedAt:         now,
	}
	if opts != nil {
		cc.AllowedAudiences = opts.AllowedAudiences
		cc.AllowedScopes = opts.AllowedScopes
		cc.AllowedActorKinds = opts.AllowedActorKinds
		cc.ExpiresAt = opts.ExpiresAt
	}
	b, _ := json.Marshal(cc)

	rec := &dbsetting.PluginInstanceConfig{
		TenantID:  tenantID,
		PluginID:  pluginID,
		Key:       KeyClientCredentials,
		ValueJSON: datatypes.JSON(b),
		Enabled:   true,
	}
	if err := s.PluginInstanceRepo.Upsert(ctx, rec); err != nil {
		return "", "", err
	}
	return clientID, clientSecretPlain, nil
}

// RotateSecret：轮换并返回新的“明文 secret”，立即替换旧 hash（如需双活，可扩展为存多版本）
func (s *PluginInstanceConfigService) RotateSecret(ctx context.Context, tenantID uint64, pluginID string) (newSecret string, err error) {
	cfg, err := s.PluginInstanceRepo.Get(ctx, tenantID, pluginID, KeyClientCredentials)
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return "", errors.New("credentials not found")
	}

	var cc ClientCredential
	if err := json.Unmarshal(cfg.ValueJSON, &cc); err != nil {
		return "", err
	}

	newSecret = utils.RandomString(48)
	hash, err := bcrypt.GenerateFromPassword([]byte(newSecret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	cc.ClientSecretHash = string(hash)
	cc.SecretVersion++
	now := time.Now().Unix()
	cc.RotatedAt = &now

	b, _ := json.Marshal(cc)
	cfg.ValueJSON = datatypes.JSON(b)
	if err := s.PluginInstanceRepo.Upsert(ctx, cfg); err != nil {
		return "", err
	}
	return newSecret, nil
}

// VerifyClient：校验 client_id/secret，并可选校验 audience/scope/actorKind
func (s *PluginInstanceConfigService) VerifyClient(
	ctx context.Context,
	tenantID uint64,
	pluginID, clientID, clientSecret, wantAudience, wantScope, wantActorKind string,
) error {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return errors.New("missing client credentials")
	}
	cfg, err := s.PluginInstanceRepo.Get(ctx, tenantID, pluginID, KeyClientCredentials)
	if err != nil {
		return err
	}
	if cfg == nil || !cfg.Enabled {
		return errors.New("plugin disabled or credentials not found")
	}

	var cc ClientCredential
	if err := json.Unmarshal(cfg.ValueJSON, &cc); err != nil {
		return err
	}
	// 到期
	if cc.ExpiresAt != nil && time.Now().Unix() > *cc.ExpiresAt {
		return errors.New("client credentials expired")
	}
	// client_id 匹配
	if cc.ClientID != clientID {
		return errors.New("invalid client_id or secret")
	}
	// 密码学校验
	if err := bcrypt.CompareHashAndPassword([]byte(cc.ClientSecretHash), []byte(clientSecret)); err != nil {
		return errors.New("invalid client_id or secret")
	}
	// 能力约束：aud/scope/actorKind（若配置了才校验）
	if wantAudience != "" && len(cc.AllowedAudiences) > 0 && !contains(cc.AllowedAudiences, wantAudience) {
		return errors.New("audience not allowed")
	}
	if wantScope != "" && len(cc.AllowedScopes) > 0 && !contains(cc.AllowedScopes, wantScope) {
		return errors.New("scope not allowed")
	}
	if wantActorKind != "" && len(cc.AllowedActorKinds) > 0 && !contains(cc.AllowedActorKinds, wantActorKind) {
		return errors.New("actor kind not allowed")
	}
	return nil
}

/* ==================== 小工具 ==================== */

func contains[T comparable](ss []T, x T) bool {
    for _, v := range ss {
        if v == x {
            return true
        }
    }
    return false
}

/* ============== 删除（按需彻底删除） ============== */

// DeleteCredentials 删除本租户-插件的凭证配置；soft=false 为硬删除
func (s *PluginInstanceConfigService) DeleteCredentials(ctx context.Context, tenantID uint64, pluginID string, soft bool) error {
    return s.PluginInstanceRepo.Delete(ctx, tenantID, strings.TrimSpace(pluginID), KeyClientCredentials, soft)
}
