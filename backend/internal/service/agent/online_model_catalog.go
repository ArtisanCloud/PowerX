package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modeldiscovery"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modelparams"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// OnlineModelSyncRequest 只接受受保护管理页的目录操作，不接受任意 HTTP 代理参数。
type OnlineModelSyncRequest struct {
	Env            string   `json:"env"`
	Provider       string   `json:"provider"`
	App            string   `json:"app"`
	Modality       string   `json:"modality"`
	Action         string   `json:"action"`
	Revision       string   `json:"revision"`
	Models         []string `json:"models"`
	ConfirmUnknown bool     `json:"confirm_unknown"`
}
type OnlineCatalogModel struct {
	Operations       map[string]catalog.MediaOperation `json:"operations,omitempty"`
	ParameterProfile *modelparams.Profile              `json:"parameter_profile,omitempty"`
	ID               string                            `json:"id"`
	Label            string                            `json:"label"`
	Modalities       []string                          `json:"modalities"`
	Match            string                            `json:"match"`
	ManualModalities []string                          `json:"manual_modalities"`
	// ReportedModalities 只表达供应商声明，不等于已有可执行媒体合同。
	ReportedModalities []string             `json:"reported_modalities"`
	InputModalities    []string             `json:"input_modalities,omitempty"`
	OutputModalities   []string             `json:"output_modalities,omitempty"`
	ManualConfirmed    bool                 `json:"manual_confirmed,omitempty"`
	ReviewRequired     bool                 `json:"review_required,omitempty"`
	Vision             *catalog.VisionUsage `json:"vision,omitempty"`
}
type OnlineModelCatalog struct {
	Provider       string               `json:"provider"`
	Env            string               `json:"env"`
	App            string               `json:"app"`
	Protocol       string               `json:"protocol"`
	Source         string               `json:"source"`
	UpdatedAt      string               `json:"updated_at"`
	Revision       string               `json:"revision"`
	Models         []OnlineCatalogModel `json:"models"`
	CorrectedCount int                  `json:"corrected_count,omitempty"`
}
type storedOnlineCatalog struct {
	OnlineModelCatalog
	Fingerprint string `json:"connection_fingerprint"`
}

func onlineCatalogKey(env, provider, app string) string {
	digest := sha256.Sum256([]byte(env + "\x00" + provider + "\x00" + app))
	return "ai.online_models." + hex.EncodeToString(digest[:])
}
func connectionFingerprint(in modeldiscovery.Input) string {
	in.Timeout = 0 // 查询期限不是连接身份；调整期限不废弃同一账号的目录。
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func syncError(status int, code string) error { return modeldiscovery.Error(status, code, nil) }

// savedCatalogConnection 读取同一租户、环境、供应商的一份凭据快照。
func (s *AgentSettingService) savedCatalogConnection(ctx context.Context, env, tenant, provider string) (modeldiscovery.Input, error) {
	in := modeldiscovery.Input{Provider: provider}
	if s == nil || s.db == nil || tenant == "" || env == "" {
		return in, syncError(409, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
	}
	in.Timeout = s.modelDiscoveryTimeout
	cred, err := s.credRepo.FindByEquivalentScopeNameProvider(ctx, env, &tenant, utils.Slug(env+"-"+provider), provider)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return in, syncError(409, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
	}
	if err != nil {
		return in, syncError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	in.Endpoint, in.APIKey, err = s.resolveConnFromCredential(ctx, env, &tenant, provider, cred, "", "")
	if err != nil {
		return in, syncError(409, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
	}
	// 兼容服务的 Key 可选；存在时仍从当前连接解封并传递，不能因 auth=none 丢弃。
	if provider == "openai_compatible" {
		if cred.Data["__sealed"] != nil {
			secret := map[string]any{}
			if err := s.tks.UnsealSensitive(ctx, cred.Env, tenant, cred.Data, &secret); err != nil {
				return in, syncError(409, "AI_MODEL_DISCOVERY_CONFIG_INVALID")
			}
			in.APIKey, _ = secret["api_key"].(string)
		} else {
			in.APIKey, _ = cred.Data["api_key"].(string)
		}
	}
	in.Organization, _ = cred.Data["organization"].(string)
	in.AnthropicVersion, _ = cred.Data["anthropic_version"].(string)
	if deployment, _ := cred.Data["azure_deployment"].(string); strings.TrimSpace(deployment) != "" {
		return in, syncError(422, "AI_MODEL_SYNC_UNSUPPORTED")
	}
	return in, nil
}

func decodeOnlineCatalog(item *dbsetting.TenantSetting) (*storedOnlineCatalog, error) {
	if item == nil {
		return nil, nil
	}
	var value storedOnlineCatalog
	if json.Unmarshal(item.ValueJSON, &value) != nil || value.Revision == "" || value.Models == nil {
		return nil, syncError(503, "AI_MODEL_SYNC_SNAPSHOT_INVALID")
	}
	return &value, nil
}

// SyncOnlineModels 抓取完整清单后原子保存；不会写源码目录或改变任何活动模型。
func (s *AgentSettingService) SyncOnlineModels(ctx context.Context, tenant string, in OnlineModelSyncRequest) (*OnlineModelCatalog, error) {
	reg := catalog.GetGlobalAIRegister()
	in.Env = dbmodel.CanonicalAIEnvironment(in.Env)
	in.App = strings.TrimSpace(in.App)
	in.Modality = catalog.NormalizeModality(in.Modality)
	in.Provider = reg.CanonicalProvider(in.Provider)
	manifest, ok := reg.Manifest(in.Provider)
	if !ok || manifest == nil || in.Env == "" || len(in.Env) > 32 || strings.TrimSpace(tenant) == "" {
		return nil, syncError(422, "AI_MODEL_SYNC_REQUEST_INVALID")
	}
	if in.App != "" {
		if _, ok := manifest.Apps[in.App]; !ok {
			return nil, syncError(422, "AI_SETTINGS_APP_INVALID")
		}
	} else if len(manifest.Apps) > 0 {
		return nil, syncError(422, "AI_SETTINGS_APP_REQUIRED")
	}
	if _, ok := modeldiscovery.ProviderSpec(in.Provider); !ok {
		return nil, syncError(422, "AI_MODEL_SYNC_UNSUPPORTED")
	}
	if in.Action != "refresh" && in.Action != "assign" && in.Action != "unassign" {
		return nil, syncError(422, "AI_MODEL_SYNC_REQUEST_INVALID")
	}
	if in.Action != "refresh" && (in.Revision == "" || len(in.Models) != 1 || (in.Modality != "llm" && in.Modality != "embedding") || manifest.Drivers[in.Modality] == "") {
		return nil, syncError(422, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
	}
	if in.Action == "assign" && !in.ConfirmUnknown {
		return nil, syncError(422, "AI_MODEL_SYNC_CONFIRMATION_REQUIRED")
	}
	conn, err := s.savedCatalogConnection(ctx, in.Env, tenant, in.Provider)
	if err != nil {
		return nil, err
	}
	fingerprint := connectionFingerprint(conn)
	var remote *modeldiscovery.Result
	if in.Action == "refresh" {
		remote, err = modeldiscovery.Fetch(ctx, conn)
		if err != nil {
			details := dto.DetailsOf(err)
			logger.WarnF(ctx, "[ai.settings] online model discovery failed tenant=%s env=%s provider=%s code=%s source=%v upstream_status=%v upstream_reason=%v", tenant, in.Env, in.Provider, dto.CodeOf(err), details["source"], details["upstream_status"], details["upstream_reason"])
			return nil, err
		}
	}
	var saved storedOnlineCatalog
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := settingrepo.NewTenantSettingRepository(tx)
		key := onlineCatalogKey(in.Env, in.Provider, in.App)
		item, e := findOnlineCatalogSetting(ctx, repo, tenant, in.Env, in.Provider, in.App, true)
		if e != nil {
			return e
		}
		previous, e := decodeOnlineCatalog(item)
		if e != nil {
			return e
		}
		// 连接在抓取期间被改动，不能把旧账号的清单写入新连接。
		current, e := NewAgentSettingService(tx).savedCatalogConnection(ctx, in.Env, tenant, in.Provider)
		if e != nil {
			return e
		}
		if connectionFingerprint(current) != fingerprint {
			return syncError(409, "AI_MODEL_SYNC_CONNECTION_CHANGED")
		}
		if in.Action == "refresh" {
			saved = storedOnlineCatalog{OnlineModelCatalog: OnlineModelCatalog{Provider: in.Provider, Env: in.Env, App: in.App, Protocol: remote.Protocol, Source: remote.Source, UpdatedAt: time.Now().UTC().Format(time.RFC3339), Models: []OnlineCatalogModel{}}, Fingerprint: fingerprint}
			manual := map[string]OnlineCatalogModel{}
			if previous != nil && previous.Fingerprint == fingerprint {
				for _, m := range previous.Models {
					manual[m.ID] = m
				}
			}
			for _, m := range remote.Models {
				entry := manual[m.ID]
				entry.ID, entry.Label, entry.ReportedModalities = m.ID, m.Label, m.Modalities
				entry.InputModalities, entry.OutputModalities = m.InputModalities, m.OutputModalities
				entry.ParameterProfile = m.ParameterProfile
				entry, corrected := classifyOnlineModel(reg, manifest, in.Provider, in.App, entry)
				if corrected {
					saved.CorrectedCount++
				}
				saved.Models = append(saved.Models, entry)
			}
		} else {
			if previous == nil || previous.Fingerprint != fingerprint || previous.Revision != in.Revision {
				return syncError(409, "AI_MODEL_SYNC_STALE")
			}
			saved = *previous
			saved.CorrectedCount = 0
			for i, entry := range saved.Models {
				classified, corrected := classifyOnlineModel(reg, manifest, in.Provider, in.App, entry)
				saved.Models[i] = classified
				if corrected {
					saved.CorrectedCount++
				}
			}
			requested := map[string]bool{}
			for _, id := range in.Models {
				requested[id] = true
			}
			for i := range saved.Models {
				entry := &saved.Models[i]
				if !requested[entry.ID] {
					continue
				}
				if len(entry.Modalities) > 0 {
					return syncError(422, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
				}
				if in.Action == "unassign" {
					if !containsModality(entry.ManualModalities, in.Modality) {
						return syncError(422, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
					}
					entry.ManualModalities = []string{}
					entry.ManualConfirmed, entry.ReviewRequired, entry.Match = false, false, "unmatched"
				} else {
					if len(entry.ReportedModalities) > 0 && !containsModality(entry.ReportedModalities, in.Modality) {
						return syncError(422, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
					}
					entry.ManualModalities = []string{in.Modality}
					entry.ManualConfirmed, entry.ReviewRequired, entry.Match = true, false, "manual"
				}
				delete(requested, entry.ID)
			}
			if len(requested) > 0 {
				return syncError(422, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
			}
		}
		saved.Env = in.Env
		saved.Revision = uuid.NewString()
		raw, e := json.Marshal(saved)
		if e != nil {
			return e
		}
		if e = repo.Upsert(ctx, &dbsetting.TenantSetting{TenantUUID: tenant, Key: key, ValueJSON: datatypes.JSON(raw), Group: "ai", Description: stringPointer("供应商在线模型清单；不含连接密钥"), Editable: false}); e != nil {
			return e
		}
		// 从存储回读，避免把内存中的结果误当成保存完成。
		readback, e := repo.GetByTenantAndKey(ctx, tenant, key)
		if e != nil {
			return e
		}
		authoritative, e := decodeOnlineCatalog(readback)
		if e != nil {
			return e
		}
		if authoritative == nil || authoritative.Revision != saved.Revision {
			return syncError(503, "AI_MODEL_SYNC_SAVE_FAILED")
		}
		saved = *authoritative
		return nil
	})
	if err != nil {
		return nil, err
	}
	logger.InfoF(ctx, "[ai.model_catalog] action=%s tenant=%s env=%s provider=%s app=%s models=%d actor_member=%d revision=%s", in.Action, tenant, in.Env, in.Provider, in.App, len(saved.Models), reqctx.GetMemberID(ctx), saved.Revision)
	enrichVisionUsage(in.Provider, saved.Models)
	return &saved.OnlineModelCatalog, nil
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func (s *AgentSettingService) onlineModels(ctx context.Context, env string, tenant *string, mod, provider, app string) ([]string, bool, error) {
	if _, supported := modeldiscovery.ProviderSpec(provider); !supported {
		return nil, false, nil
	}
	if s == nil || s.db == nil || tenant == nil || strings.TrimSpace(*tenant) == "" {
		return nil, false, nil
	}
	item, err := findOnlineCatalogSetting(ctx, s.settingRepo, *tenant, env, provider, app, false)
	if err != nil {
		return nil, false, err
	}
	snapshot, err := decodeOnlineCatalog(item)
	if err != nil {
		return nil, false, err
	}
	conn, err := s.savedCatalogConnection(ctx, env, *tenant, provider)
	if err != nil {
		return nil, false, err
	}
	if snapshot == nil || snapshot.Fingerprint != connectionFingerprint(conn) {
		fresh, fetchErr := s.SyncOnlineModels(ctx, *tenant, OnlineModelSyncRequest{Env: env, Provider: provider, App: app, Modality: mod, Action: "refresh"})
		if fetchErr != nil {
			return nil, true, fetchErr
		}
		snapshot = &storedOnlineCatalog{OnlineModelCatalog: *fresh, Fingerprint: connectionFingerprint(conn)}
	}
	models := []string{}
	for _, stored := range snapshot.Models {
		// 型号存在性以当前连接的在线结果为准；用途元数据独立展示，不截断在线清单。
		models = appendUnique(models, stored.ID)
	}
	return models, true, nil
}

func (s *AgentSettingService) ensureTenantModelExists(ctx context.Context, env string, tenant *string, mod, provider, model string) error {
	initial := ensureModelExists(mod, provider, model)
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	app, id := splitAppModel(reg, provider, model)
	if initial == nil {
		// 允许首次保存连接及已有未同步配置；已有在线快照时不再让本地型号绕过在线存在性。
		if _, supported := modeldiscovery.ProviderSpec(provider); !supported || tenant == nil || s == nil || s.db == nil {
			return nil
		}
		item, err := findOnlineCatalogSetting(ctx, s.settingRepo, *tenant, env, provider, app, false)
		if err != nil {
			return err
		}
		snapshot, err := decodeOnlineCatalog(item)
		if err != nil {
			return err
		}
		if snapshot == nil {
			return nil
		}
	}
	models, _, err := s.onlineModels(ctx, env, tenant, mod, provider, app)
	if err != nil {
		return err
	}
	for _, candidate := range models {
		if candidate == id {
			if mod == "vlm" {
				item, e := findOnlineCatalogSetting(ctx, s.settingRepo, *tenant, env, provider, app, false)
				if e != nil {
					return e
				}
				snapshot, e := decodeOnlineCatalog(item)
				if e != nil {
					return e
				}
				if snapshot != nil {
					for _, entry := range snapshot.Models {
						if entry.ID == id {
							usage := resolveVisionUsage(provider, entry)
							if usage.Status == "unsupported" {
								return &dto.AppError{HTTPCode: 422, Code: usage.Reason, Message: usage.Reason, Details: map[string]any{"provider": provider, "model": id, "modality": mod, "vision": usage}}
							}
						}
					}
				}
			}
			if manifest, ok := reg.Manifest(provider); ok && manifest.Drivers[mod] != "" {
				return nil
			}
		}
	}
	if initial == nil {
		return syncError(422, "AI_MODEL_NOT_IN_ONLINE_CATALOG")
	}
	return initial
}

func stringPointer(value string) *string { return &value }
