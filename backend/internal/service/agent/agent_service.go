// internal/service/agent/agent_service.go
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AgentService struct {
	db         *gorm.DB
	agRepo     *repo.AgentRepository
	setRepo    *repo.AgentSettingRepository
	kbRepo     *repo.AgentKBBindingRepository
	pluginRepo *repo.AgentPluginLinkRepository
}

var ErrAgentOwnerForbidden = errors.New("agent.owner_forbidden")

func NewAgentService(db *gorm.DB) *AgentService {
	return &AgentService{
		db:         db,
		agRepo:     repo.NewAgentRepository(db),
		setRepo:    repo.NewAgentSettingRepository(db),
		kbRepo:     repo.NewAgentKBBindingRepository(db),
		pluginRepo: repo.NewAgentPluginLinkRepository(db),
	}
}

// ---------- Agent CRUD ----------

func (s *AgentService) List(
	ctx context.Context,
	env string,
	tenantUUID *string,
	ownerPluginID string,
	statuses ...string,
) ([]dbmodel.Agent, error) {
	return s.agRepo.ListByScope(ctx, env, tenantUUID, statuses, ownerPluginID)
}

func (s *AgentService) Create(ctx context.Context, env string, tenantUUID *string, in *dbmodel.Agent) (*dbmodel.Agent, error) {
	in.Env, in.TenantUUID = env, tenantUUID
	if err := s.db.WithContext(ctx).Create(in).Error; err != nil {
		return nil, err
	}
	// 跟创默认空配置（幂等）
	_ = s.ensureDefaultAgentSetting(ctx, env, tenantUUID, in.ID)
	return in, nil
}

// 确保存在一条默认的 AgentSetting（空 provider/model，health=unknown）
func (s *AgentService) ensureDefaultAgentSetting(ctx context.Context, env string, tenantUUID *string, agentID uint64) error {
	// 已存在就跳过
	if _, err := s.setRepo.FindByScopeAgentID(ctx, env, tenantUUID, agentID); err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// 插入默认记录
	rec := &dbmodel.AgentSetting{
		Env:           env,
		TenantUUID:    tenantUUID,
		AgentID:       agentID,
		Provider:      "",
		Model:         "",
		Params:        datatypes.JSONMap{},
		OverrideFlags: datatypes.JSONMap{},
		QuotaPolicy:   datatypes.JSONMap{},
		HealthStatus:  "unknown",
		HealthInfo:    datatypes.JSONMap{},
	}
	return s.setRepo.UpsertByScopeAgentID(ctx, rec)
}

type AgentPatch struct {
	Name                  *string
	Description           *string
	Visibility            *string
	Status                *string
	Scope                 *string
	DefaultPersonaID      *uint64
	BlueprintRefs         datatypes.JSON
	IntentCardsRef        datatypes.JSON
	ToolAllowlist         datatypes.JSON
	KBStrategy            *string
	Meta                  datatypes.JSONMap
	ExpectedOwnerPluginID *string
	CallerPluginID        string
}

func (s *AgentService) Update(ctx context.Context, env string, tenantUUID *string, agentID uint64, patch AgentPatch) (*dbmodel.Agent, error) {
	exist, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	// 租户隔离（简单校验）
	if !equalTenant(tenantUUID, exist.TenantUUID) {
		return nil, gorm.ErrRecordNotFound
	}
	if err := assertPluginOwnership(exist, patch.ExpectedOwnerPluginID, patch.CallerPluginID); err != nil {
		return nil, err
	}

	// 组装要更新字段
	up := map[string]any{}
	if patch.Name != nil {
		up["name"] = *patch.Name
	}
	if patch.Description != nil {
		up["description"] = *patch.Description
	}
	if patch.Visibility != nil {
		up["visibility"] = *patch.Visibility
	}
	if patch.Status != nil {
		up["status"] = *patch.Status
	}
	if patch.Scope != nil {
		up["scope"] = *patch.Scope
	}
	if patch.DefaultPersonaID != nil {
		up["default_persona_id"] = patch.DefaultPersonaID
	}
	if patch.BlueprintRefs != nil {
		up["blueprint_refs"] = patch.BlueprintRefs
	}
	if patch.IntentCardsRef != nil {
		up["intent_cards_ref"] = patch.IntentCardsRef
	}
	if patch.ToolAllowlist != nil {
		up["tool_allowlist"] = patch.ToolAllowlist
	}
	if patch.KBStrategy != nil {
		up["kb_strategy"] = *patch.KBStrategy
	}
	if patch.Meta != nil {
		up["meta"] = patch.Meta
	}

	if len(up) > 0 {
		if err := s.db.WithContext(ctx).Model(&dbmodel.Agent{}).Where("id = ?", agentID).Updates(up).Error; err != nil {
			return nil, err
		}
	}
	return s.agRepo.GetByID(ctx, agentID)
}

func (s *AgentService) SetStatus(ctx context.Context, env string, tenantUUID *string, agentID uint64, status string) error {
	exist, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if !equalTenant(tenantUUID, exist.TenantUUID) {
		return gorm.ErrRecordNotFound
	}
	return s.agRepo.UpdateStatus(ctx, agentID, status)
}

func (s *AgentService) Delete(ctx context.Context, env string, tenantUUID *string, agentID uint64, expectedOwnerPluginID *string, callerPluginID string) error {
	exist, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if !equalTenant(tenantUUID, exist.TenantUUID) {
		return gorm.ErrRecordNotFound
	}
	if err := assertPluginOwnership(exist, expectedOwnerPluginID, callerPluginID); err != nil {
		return err
	}
	// 保护：内置不可删
	if v, ok := exist.Meta["protect_from_delete"]; ok {
		if vb, ok2 := v.(bool); ok2 && vb {
			return errors.New("内置系统 Agent 禁止删除")
		}
	}
	return s.agRepo.DeleteSoft(ctx, agentID)
}

func (s *AgentService) Get(ctx context.Context, env string, tenantUUID *string, agentID uint64) (*dbmodel.Agent, error) {
	out, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if !equalTenant(tenantUUID, out.TenantUUID) {
		return nil, gorm.ErrRecordNotFound
	}
	return out, nil
}

func (s *AgentService) GetByUUID(ctx context.Context, env string, tenantUUID *string, agentUUID uuid.UUID) (*dbmodel.Agent, error) {
	out, err := s.agRepo.FindByScopeUUID(ctx, env, tenantUUID, agentUUID)
	if err != nil {
		return nil, err
	}
	if !equalTenant(tenantUUID, out.TenantUUID) {
		return nil, gorm.ErrRecordNotFound
	}
	return out, nil
}

//func (s *AgentService) List(
//	ctx context.Context, env string, tenantUUID *string, statuses ...string,
//) ([]dbmodel.Agent, error) {
//	// statuses 在这里是 []string
//	return s.agRepo.ListByScope(ctx, env, tenantUUID, statuses...)
//}

// ---------- Agent Setting（Agent级 AI 覆盖） ----------

func (s *AgentService) GetAgentAISetting(ctx context.Context, env string, tenantUUID *string, agentID uint64) (*dbmodel.AgentSetting, error) {
	rec, err := s.setRepo.FindByScopeAgentID(ctx, env, tenantUUID, agentID)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// 自动补齐
	if e := s.ensureDefaultAgentSetting(ctx, env, tenantUUID, agentID); e != nil {
		return nil, e
	}
	return s.setRepo.FindByScopeAgentID(ctx, env, tenantUUID, agentID)
}

// PATCH/PUT upsert：直接落库（前面 handler 已做校验/规范化）
func (s *AgentService) UpsertAgentAISetting(ctx context.Context, env string, tenantUUID *string, in *dbmodel.AgentSetting) (*dbmodel.AgentSetting, error) {
	in.Env, in.TenantUUID = env, tenantUUID
	if err := s.setRepo.UpsertByScopeAgentID(ctx, in); err != nil {
		return nil, err
	}
	return s.setRepo.FindByScopeAgentID(ctx, env, tenantUUID, in.AgentID)
}

// DELETE
func (s *AgentService) DeleteAgentAISetting(ctx context.Context, env string, tenantUUID *string, agentID uint64) error {
	return s.setRepo.DeleteByScopeAgentID(ctx, env, tenantUUID, agentID)
}

// 健康检查（最小实现：检查是否具备可解析的 provider/model；真正连通可复用 SettingHandler 的 svc.PingLLM）
// 健康检查：这里简单从设置里拿 provider/model 做个占位（如需真正 ping，可复用你已有的 LLM ping）
func (s *AgentService) HealthCheck(ctx context.Context, env string, tenantUUID *string, agentID uint64) (map[string]any, error) {
	rec, err := s.setRepo.FindByScopeAgentID(ctx, env, tenantUUID, agentID)
	if err != nil {
		return nil, err
	}
	// 这里可以调用你之前的 PingLLM / TestConnectionPreferInput 等
	// 为简洁先返回静态占位
	return map[string]any{
		"status":   rec.HealthStatus,
		"info":     rec.HealthInfo,
		"provider": rec.Provider,
		"model":    rec.Model,
	}, nil
}

// ---------- helpers ----------
func equalTenant(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(*a) == strings.TrimSpace(*b)
}

func assertPluginOwnership(rec *dbmodel.Agent, expectedOwnerPluginID *string, callerPluginID string) error {
	owner := strings.TrimSpace(pluginOwner(rec))
	if expectedOwnerPluginID != nil {
		expected := strings.TrimSpace(*expectedOwnerPluginID)
		if expected == "" {
			if owner != "" {
				return ErrAgentOwnerForbidden
			}
		} else if !strings.EqualFold(expected, owner) {
			return fmt.Errorf("%w: expected=%s actual=%s", ErrAgentOwnerForbidden, expected, owner)
		}
	}
	callerPluginID = strings.TrimSpace(callerPluginID)
	if callerPluginID == "" {
		return nil
	}
	if owner == "" || !strings.EqualFold(owner, callerPluginID) {
		return fmt.Errorf("%w: caller=%s actual=%s", ErrAgentOwnerForbidden, callerPluginID, owner)
	}
	return nil
}

func pluginOwner(rec *dbmodel.Agent) string {
	if rec == nil {
		return ""
	}
	if rec.OwnerPluginID != nil && strings.TrimSpace(*rec.OwnerPluginID) != "" {
		return strings.TrimSpace(*rec.OwnerPluginID)
	}
	src := strings.TrimSpace(rec.Source)
	if strings.HasPrefix(strings.ToLower(src), "plugin:") {
		return strings.TrimSpace(src[len("plugin:"):])
	}
	return ""
}
