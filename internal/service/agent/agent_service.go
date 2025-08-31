// internal/service/agent/agent_service.go
package agent

import (
	"context"
	"errors"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"

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

func (s *AgentService) Create(ctx context.Context, env string, tenantID *uint64, in *dbmodel.Agent) (*dbmodel.Agent, error) {
	if tenantID == nil {
		return nil, errors.New("tenantID 不能为空")
	}
	if in == nil {
		return nil, errors.New("payload 为空")
	}
	if err := s.agRepo.UpsertByScopeKey(ctx, env, tenantID, in); err != nil {
		return nil, err
	}
	// 再查一次保证主键正确
	out, err := s.agRepo.FindByScopeKey(ctx, env, tenantID, in.Key)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type AgentPatch struct {
	Name             *string
	Description      *string
	Visibility       *string
	Status           *string
	Scope            *string
	DefaultPersonaID *uint64
	BlueprintRefs    datatypes.JSON
	IntentCardsRef   datatypes.JSON
	ToolAllowlist    []string
	KBStrategy       *string
	Meta             datatypes.JSONMap
}

func (s *AgentService) Update(ctx context.Context, env string, tenantID *uint64, agentID uint64, patch AgentPatch) (*dbmodel.Agent, error) {
	exist, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	// 租户隔离（简单校验）
	if !equalTenant(tenantID, exist.TenantID) {
		return nil, gorm.ErrRecordNotFound
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

func (s *AgentService) SetStatus(ctx context.Context, env string, tenantID *uint64, agentID uint64, status string) error {
	exist, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if !equalTenant(tenantID, exist.TenantID) {
		return gorm.ErrRecordNotFound
	}
	return s.agRepo.UpdateStatus(ctx, agentID, status)
}

func (s *AgentService) Delete(ctx context.Context, env string, tenantID *uint64, agentID uint64) error {
	exist, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if !equalTenant(tenantID, exist.TenantID) {
		return gorm.ErrRecordNotFound
	}
	// 保护：内置不可删
	if v, ok := exist.Meta["protect_from_delete"]; ok {
		if vb, ok2 := v.(bool); ok2 && vb {
			return errors.New("内置系统 Agent 禁止删除")
		}
	}
	return s.agRepo.DeleteSoft(ctx, agentID)
}

func (s *AgentService) Get(ctx context.Context, env string, tenantID *uint64, agentID uint64) (*dbmodel.Agent, error) {
	out, err := s.agRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if !equalTenant(tenantID, out.TenantID) {
		return nil, gorm.ErrRecordNotFound
	}
	return out, nil
}

func (s *AgentService) List(ctx context.Context, env string, tenantID *uint64, statuses ...string) ([]dbmodel.Agent, error) {
	return s.agRepo.ListByScope(ctx, env, tenantID, statuses...)
}

// ---------- Agent Setting（Agent级 AI 覆盖） ----------

func (s *AgentService) GetAgentAISetting(ctx context.Context, env string, tenantID *uint64, agentID uint64) (*dbmodel.AgentSetting, error) {
	// 基本存在性 & 租户校验
	if _, err := s.Get(ctx, env, tenantID, agentID); err != nil {
		return nil, err
	}
	// 没有记录时，返回 ErrRecordNotFound（前端可理解为“使用上游默认”）
	return s.setRepo.FindByAgent(ctx, env, tenantID, agentID)
}

func (s *AgentService) UpsertAgentAISetting(ctx context.Context, env string, tenantID *uint64, in *dbmodel.AgentSetting) (*dbmodel.AgentSetting, error) {
	// 基本存在性 & 租户校验
	if _, err := s.Get(ctx, env, tenantID, in.AgentID); err != nil {
		return nil, err
	}
	if err := s.setRepo.UpsertByAgent(ctx, env, tenantID, in); err != nil {
		return nil, err
	}
	return s.setRepo.FindByAgent(ctx, env, tenantID, in.AgentID)
}

func (s *AgentService) DeleteAgentAISetting(ctx context.Context, env string, tenantID *uint64, agentID uint64) error {
	// 存在性 & 租户校验
	if _, err := s.Get(ctx, env, tenantID, agentID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("env=? AND tenant_id=? AND agent_id=?", env, *tenantID, agentID).
		Delete(&dbmodel.AgentSetting{}).Error
}

// 健康检查（最小实现：检查是否具备可解析的 provider/model；真正连通可复用 SettingHandler 的 svc.PingLLM）
func (s *AgentService) HealthCheck(ctx context.Context, env string, tenantID *uint64, agentID uint64) (map[string]any, error) {
	set, err := s.setRepo.FindByAgent(ctx, env, tenantID, agentID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	info := map[string]any{"agentId": agentID}
	if set == nil || strings.TrimSpace(set.Provider) == "" || strings.TrimSpace(set.Model) == "" {
		info["level"] = "missing"
		return info, errors.New("缺少 agent 级 AISetting（将回退到上游默认）")
	}
	info["provider"] = set.Provider
	info["model"] = set.Model
	info["level"] = "agent_override"
	return info, nil
}

// ---------- helpers ----------
func equalTenant(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
