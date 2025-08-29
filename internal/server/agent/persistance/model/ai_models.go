package model

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ---------- 表名常量 ----------
const (
	TableAIProviderCredential = "ai_provider_credentials"
	TableAIModelProfile       = "ai_model_profiles"
	TableAIRoutePolicy        = "ai_route_policies"
	TableAIUsageLog           = "ai_usage_logs"
)

// 1) 凭据
type AIProviderCredential struct {
	coremodel.PowerModel `json:"-"` // 基础字段已带 json 标签
	coremodel.ScopeRef   `json:"-"` // Env / TenantID

	// 逻辑名，用来在策略里引用
	Name     string `gorm:"size:128;index:uniq_cred,unique" json:"name"`     // e.g. "openai-prod-tokyo"
	Provider string `gorm:"size:64;index:uniq_cred,unique"  json:"provider"` // openai|azure|baidu|zhipu|...

	// 鉴权方案及数据（建议只存 secret 引用）
	AuthScheme string            `gorm:"size:32"                         json:"authScheme"` // bearer|aksk|oauth|query_token|custom_sig
	Data       datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb"  json:"data"`       // {api_key, base_url, region, project_id,...}
}

// 带 schema 的表名
func (mdl *AIProviderCredential) TableName() string {
	return coremodel.PowerXSchema + "." + TableAIProviderCredential
}
func (mdl *AIProviderCredential) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAIProviderCredential
}

// 2) 模型画像
type AIModelProfile struct {
	coremodel.PowerModel `json:"-"`
	coremodel.ScopeRef   `json:"-"`

	Modality string `gorm:"size:32;index:uniq_prof,unique" json:"modality"` // llm|image|embedding|audio_tts|audio_asr|video
	Provider string `gorm:"size:64;index:uniq_prof,unique" json:"provider"`
	Model    string `gorm:"size:128;index:uniq_prof,unique" json:"model"` // e.g. gpt-4o-mini

	// 画像名（可选），便于人类识别
	Label string `gorm:"size:128" json:"label"`

	// 默认参数（温度/尺寸/dims/格式/语言…）
	Defaults datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"defaults"`

	// 能力缓存（探针结果）
	CapCache datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"capCache"`

	// 标签：cheap/fast/vision/json/cn/us…
	Tags datatypes.JSONSlice[string] `gorm:"type:jsonb;default:'[]'::jsonb" json:"tags"`
}

func (mdl *AIModelProfile) TableName() string {
	return coremodel.PowerXSchema + "." + TableAIModelProfile
}
func (mdl *AIModelProfile) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAIModelProfile
}

// 3) 路由策略
type AIRoutePolicy struct {
	coremodel.PowerModel `json:"-"`
	coremodel.ScopeRef   `json:"-"`

	Modality string `gorm:"size:32;index" json:"modality"` // llm|image|...

	// 选择器：可为空；不空时用于精确命中
	AgentID *string `gorm:"size:64;index" json:"agentId,omitempty"`
	FlowID  *string `gorm:"size:64;index" json:"flowId,omitempty"`
	Purpose *string `gorm:"size:32;index" json:"purpose,omitempty"` // json|vision|tool|general...

	// 策略体：候选列表/回退/权重/超时
	Strategy datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"strategy"`

	// 合规/地域/屏蔽项
	Compliance datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"compliance"`

	// 配额/费用
	Quota datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"quota"`
}

func (mdl *AIRoutePolicy) TableName() string {
	return coremodel.PowerXSchema + "." + TableAIRoutePolicy
}
func (mdl *AIRoutePolicy) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAIRoutePolicy
}

// 4) 用量日志（推荐）
type AIUsageLog struct {
	coremodel.PowerModel `json:"-"`
	coremodel.ScopeRef   `json:"-"`

	Modality string `gorm:"size:32;index"  json:"modality"`
	Provider string `gorm:"size:64;index"  json:"provider"`
	Model    string `gorm:"size:128;index" json:"model"`

	AgentID *string `gorm:"size:64;index" json:"agentId,omitempty"`
	FlowID  *string `gorm:"size:64;index" json:"flowId,omitempty"`

	CostUSD   float64 `json:"costUSD"`
	LatencyMS int     `json:"latencyMS"`
	TokensIn  int     `json:"tokensIn"`
	TokensOut int     `json:"tokensOut"`

	Meta datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"meta"`
}

func (mdl *AIUsageLog) TableName() string {
	return coremodel.PowerXSchema + "." + TableAIUsageLog
}
func (mdl *AIUsageLog) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAIUsageLog
}

// 作用域查询（PostgreSQL 建议使用 IS NOT DISTINCT FROM 处理 NULL 比较）
func WithScope(env string, tenantID *uint64) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("env = ? AND tenant_id IS NOT DISTINCT FROM ?", env, tenantID)
	}
}
