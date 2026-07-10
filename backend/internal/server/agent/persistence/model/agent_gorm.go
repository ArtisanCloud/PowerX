// pkg/corex/agent/persistence/model/agent_gorm.go
package model

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ---------- 表名常量 ----------
const (
	TableAgent                 = "agents"
	TableAgentSetting          = "agent_settings"
	TableAgentKBBinding        = "agent_kb_bindings"
	TableAgentSkillBinding     = "agent_skill_bindings"
	TableAgentKnowledgeBinding = "agent_knowledge_bindings"
	TableAgentPluginLink       = "agent_plugin_links"
	TableAgentCapabilityGrant  = "agent_capability_grants"

	TableAgentChatSession       = "agent_chat_sessions"
	TableAgentChatMessage       = "agent_chat_messages"
	TableAgentContextSummary    = "agent_chat_context_summaries"
	TableAgentSessionSkillState = "agent_session_skill_states"
	TableAgentRuntimeConfig     = "agent_runtime_configs"
)

// ---------- 枚举/常量（可按需扩展） ----------
const (
	AgentScopeSystem = "system"
	AgentScopeTenant = "tenant"

	AgentVisibilityPrivate = "private"
	AgentVisibilityTenant  = "tenant"
	AgentVisibilityPublic  = "public"

	AgentStatusDraft    = "draft"
	AgentStatusActive   = "active"
	AgentStatusDisabled = "disabled"
	AgentStatusBroken   = "broken"
	AgentStatusArchived = "archived"

	KBStrategyNone     = "none"
	KBStrategyUnion    = "union"
	KBStrategyWeighted = "weighted"

	InstallStatusInstalled = "installed"
	InstallStatusDisabled  = "disabled"
	InstallStatusBroken    = "broken"

	AgentCapabilityGrantStatusEnabled  = "enabled"
	AgentCapabilityGrantStatusDisabled = "disabled"
	AgentCapabilityGrantSourceManual   = "manual"
	AgentCapabilityGrantSourcePlugin   = "plugin_registry"
)

// ---------- 1) Agent 主表 ----------
// 说明：沿用 Env + TenantUUID 的作用域与索引策略；(Env, TenantUUID, Key) 在同一租户内唯一。
type Agent struct {
	coremodel.PowerUUIDModel

	// 作用域
	Env        string  `gorm:"size:32;index:agent_key_uniq_global,unique,priority:1,where:tenant_uuid IS NULL;index:agent_key_uniq_tenant,unique,priority:1" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_key_uniq_tenant,unique,priority:2" json:"-"` // null 表示 system 级

	// 逻辑键与可读信息
	Key         string `gorm:"size:64;index:agent_key_uniq_global,unique,priority:2,where:tenant_uuid IS NULL;index:agent_key_uniq_tenant,unique,priority:3" json:"key"`
	Name        string `gorm:"size:128" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	TypeID      string `gorm:"column:type_id;size:128;index" json:"typeId,omitempty"`
	Scene       string `gorm:"column:scene;size:128;index" json:"scene,omitempty"`
	PromptSeed  string `gorm:"column:prompt_seed;type:text" json:"promptSeed,omitempty"`
	Persona     string `gorm:"column:persona;type:text" json:"persona,omitempty"`

	// 来源/范围/可见性/状态
	Source          string  `gorm:"size:128;index" json:"source"`                               // core | plugin:<plugin_id>
	OwnerPluginID   *string `gorm:"column:owner_plugin_id;size:128;index" json:"ownerPluginId"` // 归属插件（为空表示底座）
	OwnerTenantUUID *string `gorm:"column:owner_tenant_uuid;index" json:"ownerTenantUuid"`      // 归属租户（插件托管时等于 tenant_uuid）
	ManagedByPlugin bool    `gorm:"column:managed_by_plugin;default:false;index" json:"managedByPlugin"`
	Scope           string  `gorm:"size:16;default:'tenant'" json:"scope"`            // system | tenant
	Visibility      string  `gorm:"size:16;default:'tenant';index" json:"visibility"` // private | tenant | public
	Status          string  `gorm:"size:16;default:'draft';index" json:"status"`      // draft|active|disabled|broken|archived

	// Persona / Blueprint / 工具清单
	DefaultPersonaID *uint64        `gorm:"index" json:"defaultPersonaId,omitempty"`
	BlueprintRefs    datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb" json:"blueprintRefs"`
	IntentCardsRef   datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb" json:"intentCardsRef"`
	ToolAllowlist    datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb" json:"toolAllowlist"`

	SessionSingleton bool `gorm:"default:false" json:"sessionSingleton"` // 该 Agent 是否单例会话
	DefaultTTLDays   int  `gorm:"default:3" json:"defaultTTLDays"`
	DefaultMaxKB     int  `gorm:"default:200" json:"defaultMaxKB"`
	DefaultMaxTokens int  `gorm:"default:3000" json:"defaultMaxTokens"`

	// 知识库策略与扩展元信息
	KBStrategy string            `gorm:"size:16;default:'union'" json:"kbStrategy"` // none|union|weighted
	Meta       datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"meta"`
}

func (mdl *Agent) BeforeCreate(tx *gorm.DB) error {
	if mdl.UUID == uuid.Nil {
		mdl.UUID = uuid.New()
	}
	return nil
}

// 表名（带 schema）
func (mdl *Agent) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgent
}
func (mdl *Agent) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgent
}

// ---------- 2) Agent 级 AI 覆盖设置 ----------
// 说明：通常 1:1 绑定到 Agent；包含 provider/model/参数与覆盖标记、健康状态等。
type AgentSetting struct {
	coremodel.PowerModel

	Env        string  `gorm:"size:32;index:agent_setting_uniq_global,unique,priority:1,where:tenant_uuid IS NULL;index:agent_setting_uniq_tenant,unique,priority:1" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_setting_uniq_tenant,unique,priority:2" json:"-"`

	AgentID uint64 `gorm:"index:agent_setting_uniq_global,unique,priority:2,where:tenant_uuid IS NULL;index:agent_setting_uniq_tenant,unique,priority:3" json:"agentId"`

	Provider string            `gorm:"size:64"   json:"provider"`
	Model    string            `gorm:"size:128"  json:"model"`
	Params   datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"params"` // temperature/max_tokens/top_p/stop/response_format...

	// 指定哪些字段为“强覆盖”，未标记的字段回退到租户/系统默认
	OverrideFlags datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"overrideFlags"`

	// 限流/预算等（可选）
	QuotaPolicy datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"quotaPolicy"`

	// 探针/健康状态缓存
	HealthStatus string            `gorm:"size:16;default:'unknown';index" json:"healthStatus"` // ready|degraded|unknown
	HealthInfo   datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb"   json:"healthInfo"`
}

func (mdl *AgentSetting) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentSetting
}
func (mdl *AgentSetting) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentSetting
}

// ---------- 3) Agent ↔ 知识库 绑定表 ----------
type AgentKBBinding struct {
	coremodel.PowerModel

	Env        string  `gorm:"size:32;index" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index" json:"-"`

	AgentID uint64 `gorm:"index:agent_kb_uniq,unique,priority:1" json:"agentId"`
	KBID    uint64 `gorm:"index:agent_kb_uniq,unique,priority:2" json:"kbId"`

	Weight int    `gorm:"default:1" json:"weight"`                      // weighted 策略下有效
	Status string `gorm:"size:16;default:'active';index" json:"status"` // active|disabled

	IndexPolicy datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"indexPolicy"` // 分片/召回策略等
}

func (mdl *AgentKBBinding) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentKBBinding
}
func (mdl *AgentKBBinding) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentKBBinding
}

// ---------- 4) 插件联动（可选表） ----------
type AgentPluginLink struct {
	coremodel.PowerModel

	Env        string  `gorm:"size:32;index:agent_plug_uniq,unique,priority:1" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_plug_uniq,unique,priority:2" json:"-"`

	AgentID        uint64 `gorm:"index" json:"agentId"`
	PluginID       string `gorm:"size:128;index:agent_plug_uniq,unique,priority:3" json:"pluginId"`
	PluginAgentKey string `gorm:"size:64;index:agent_plug_uniq,unique,priority:4"  json:"pluginAgentKey"` // 插件内部 agent key
	InstallStatus  string `gorm:"size:16;default:'installed';index" json:"installStatus"`                 // installed|disabled|broken
}

// ---------- 5) Agent ↔ Skill 绑定表 ----------
type AgentSkillBinding struct {
	coremodel.PowerModel

	Env        string  `gorm:"size:32;index:agent_skill_uniq,unique,priority:1" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_skill_uniq,unique,priority:2" json:"-"`

	AgentID uint64 `gorm:"column:agent_id;index:agent_skill_uniq,unique,priority:3;index" json:"agentId"`
	SkillID string `gorm:"column:skill_id;size:128;index:agent_skill_uniq,unique,priority:4;index" json:"skillId"`

	Priority int  `gorm:"column:priority;default:100" json:"priority"`
	Enabled  bool `gorm:"column:enabled;default:true;index" json:"enabled"`
}

func (mdl *AgentSkillBinding) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentSkillBinding
}
func (mdl *AgentSkillBinding) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentSkillBinding
}

// ---------- 6) Agent ↔ Knowledge Space 绑定表 ----------
type AgentKnowledgeBinding struct {
	coremodel.PowerModel

	Env        string  `gorm:"size:32;index:agent_knowledge_uniq,unique,priority:1" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_knowledge_uniq,unique,priority:2" json:"-"`

	AgentID            uint64 `gorm:"column:agent_id;index:agent_knowledge_uniq,unique,priority:3;index" json:"agentId"`
	KnowledgeSpaceUUID string `gorm:"column:knowledge_space_uuid;size:64;index:agent_knowledge_uniq,unique,priority:4;index" json:"knowledgeSpaceUuid"`

	Weight  int  `gorm:"column:weight;default:1" json:"weight"`
	Enabled bool `gorm:"column:enabled;default:true;index" json:"enabled"`
}

func (mdl *AgentKnowledgeBinding) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentKnowledgeBinding
}
func (mdl *AgentKnowledgeBinding) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentKnowledgeBinding
}

// ---------- 7) Agent Session ↔ Skill 工作状态 ----------
type AgentSessionSkillState struct {
	coremodel.PowerModel

	Env        string  `gorm:"size:32;index:agent_skill_state_uniq,unique,priority:1;index" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_skill_state_uniq,unique,priority:2;index" json:"-"`

	SessionID uint64 `gorm:"column:session_id;index:agent_skill_state_uniq,unique,priority:3;index" json:"sessionId"`
	AgentID   uint64 `gorm:"column:agent_id;index:agent_skill_state_uniq,unique,priority:4;index" json:"agentId"`
	SkillID   string `gorm:"column:skill_id;size:128;index:agent_skill_state_uniq,unique,priority:5;index" json:"skillId"`
	StateKey  string `gorm:"column:state_key;size:128;index:agent_skill_state_uniq,unique,priority:6;index" json:"stateKey"`

	SchemaVersion string            `gorm:"column:schema_version;size:32;default:'1.0'" json:"schemaVersion"`
	Status        string            `gorm:"column:status;size:32;default:'collecting';index" json:"status"`
	Action        string            `gorm:"column:action;size:64;index" json:"action,omitempty"`
	State         datatypes.JSONMap `gorm:"column:state;type:jsonb" json:"state"`
	Meta          datatypes.JSONMap `gorm:"column:meta;type:jsonb" json:"meta"`
	LastMessageID uint64            `gorm:"column:last_message_id;index" json:"lastMessageId"`
	Version       int64             `gorm:"column:version;default:1" json:"version"`
	ExpiresAt     *time.Time        `gorm:"column:expires_at;index" json:"expiresAt,omitempty"`
}

func (mdl *AgentSessionSkillState) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentSessionSkillState
}
func (mdl *AgentSessionSkillState) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentSessionSkillState
}

func (mdl *AgentPluginLink) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentPluginLink
}
func (mdl *AgentPluginLink) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentPluginLink
}

// AgentCapabilityGrant records the capabilities an Agent is explicitly allowed to use.
type AgentCapabilityGrant struct {
	coremodel.PowerUUIDModel

	Env        string `gorm:"column:env;size:32;not null;index:agent_cap_grant_uniq,unique,priority:1" json:"-"`
	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);not null;index:agent_cap_grant_uniq,unique,priority:2;index" json:"tenant_uuid"`

	AgentUUID      uuid.UUID  `gorm:"column:agent_uuid;type:uuid;not null;index:agent_cap_grant_uniq,unique,priority:3;index" json:"agent_uuid"`
	CapabilityUUID uuid.UUID  `gorm:"column:capability_uuid;type:uuid;not null;index:agent_cap_grant_uniq,unique,priority:4;index" json:"capability_uuid"`
	PluginUUID     *uuid.UUID `gorm:"column:plugin_uuid;type:uuid;index" json:"plugin_uuid,omitempty"`

	CapabilityID   string `gorm:"column:capability_id;type:varchar(128);not null;index" json:"capability_id"`
	PluginID       string `gorm:"column:plugin_id;type:varchar(128);index" json:"plugin_id,omitempty"`
	PermissionCode string `gorm:"column:permission_code;type:varchar(255);not null;index:agent_cap_grant_uniq,unique,priority:5;index" json:"permission_code"`
	RiskLevel      string `gorm:"column:risk_level;type:varchar(32);not null;default:'unknown';index" json:"risk_level"`
	Status         string `gorm:"column:status;type:varchar(16);not null;default:'enabled';index" json:"status"`
	Source         string `gorm:"column:source;type:varchar(32);not null;default:'manual';index" json:"source"`

	CreatedByUserUUID string `gorm:"column:created_by_user_uuid;type:char(36);index" json:"created_by_user_uuid,omitempty"`
	UpdatedByUserUUID string `gorm:"column:updated_by_user_uuid;type:char(36);index" json:"updated_by_user_uuid,omitempty"`
}

func (mdl *AgentCapabilityGrant) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentCapabilityGrant
}
func (mdl *AgentCapabilityGrant) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentCapabilityGrant
}
