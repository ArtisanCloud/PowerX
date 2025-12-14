package registry

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// Domain type aliases 暴露给上层调用方，避免重复结构定义。
type (
	Registration      = repo.Registration
	AdapterEndpoint   = repo.AdapterEndpoint
	EnvironmentPolicy = repo.EnvironmentPolicy
	RoutingPolicy     = repo.RoutingPolicy
	RateLimit         = repo.RateLimit
	FallbackPlan      = repo.FallbackPlan
	StaticResponse    = repo.StaticResponse
	VisibilityPolicy  = repo.VisibilityPolicy
	VisibilityRule    = repo.VisibilityRule
)

var (
	// ErrRegistrationNotFound 表示指定能力注册不存在。
	ErrRegistrationNotFound = repo.ErrNotFound
	// ErrVersionConflict 表示乐观锁冲突。
	ErrVersionConflict = repo.ErrVersionConflict
	// ErrInvalidPayload 表示请求体校验失败。
	ErrInvalidPayload = errors.New("capability registry: invalid payload")
)

// Repository 抽象底层仓储接口，便于注入内存或 GORM 实现。
type Repository interface {
	Create(ctx context.Context, db *gorm.DB, reg Registration) (Registration, error)
	Update(ctx context.Context, db *gorm.DB, reg Registration, expectedVersion uint64) (Registration, error)
	Disable(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID, reason, actor string, expectedVersion uint64, next Registration) (Registration, error)
	GetLatest(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID string) (Registration, error)
	GetVersion(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID string, version uint64) (Registration, error)
	ListLatest(ctx context.Context, db *gorm.DB, tenantUUID string, limit, offset int) ([]Registration, int64, error)
}

// ContractVerifier 校验契约是否存在且有效。
type ContractVerifier interface {
	VerifyContract(ctx context.Context, tenantUUID, contractRef string) error
}

// ToolGrantVerifier 校验 Tool Grant ID 列表是否有效。
type ToolGrantVerifier interface {
	VerifyToolGrants(ctx context.Context, tenantUUID string, grantIDs []string) error
}

// VersionGenerator 负责生成下一个版本号。
type VersionGenerator func(ctx context.Context, capabilityID, tenantUUID string, currentVersion uint64) (uint64, error)

// SystemActorResolver 根据上下文推断操作者身份。
type SystemActorResolver func(ctx context.Context) string

// ServiceOptions 注入 Service 依赖。
type ServiceOptions struct {
	DB                  *gorm.DB
	Repository          Repository
	EventBus            event_bus.EventBus
	Instrumentation     *Instrumentation
	ContractVerifier    ContractVerifier
	ToolGrantVerifier   ToolGrantVerifier
	Auditor             auditpkg.Auditor
	Clock               func() time.Time
	VersionGenerator    VersionGenerator
	SystemActorResolver SystemActorResolver
}

// CreateRegistrationInput 创建能力注册所需字段。
type CreateRegistrationInput struct {
	Registration RegistrationPayload
	Actor        string
}

// UpdateRegistrationInput 更新能力注册所需字段。
type UpdateRegistrationInput struct {
	Registration RegistrationPayload
	Actor        string
}

// DisableRegistrationInput 禁用能力注册操作字段。
type DisableRegistrationInput struct {
	CapabilityID string
	TenantUUID   string
	Reason       string
	Actor        string
	Version      uint64
}

// RegistrationPayload 提供注册信息载荷，Update 需要携带 Version。
type RegistrationPayload struct {
	CapabilityID        string
	TenantUUID          string
	ContractRef         string
	Status              string
	EnvironmentPolicies map[string]EnvironmentPolicy
	Adapters            []AdapterEndpoint
	RoutingPolicy       RoutingPolicy
	FallbackPlan        *FallbackPlan
	ToolGrantIDs        []string
	Version             uint64
	DisableReason       string
}

// GetRegistrationOptions 查询时的可选参数。
type GetRegistrationOptions struct {
	VersionSelector string // latest (default) / draft / numeric
	Version         uint64 // 当 VersionSelector 为数字时使用
	IncludeDisabled bool
}

// Instrumentation 聚合观测依赖；在 domain 包中定义。
type Instrumentation = domain.Instrumentation

func (p RegistrationPayload) canonicalTenantUUID() string {
	return strings.TrimSpace(p.TenantUUID)
}

func (in DisableRegistrationInput) canonicalTenantUUID() string {
	return strings.TrimSpace(in.TenantUUID)
}
