package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

var (
	ErrCapabilityExists     = errors.New("authorization: capability already exists")
	ErrCapabilityNotFound   = errors.New("authorization: capability not found")
	ErrGrantNotFound        = errors.New("authorization: grant not found")
	ErrChallengeNotFound    = errors.New("authorization: challenge ticket not found")
	ErrServiceUnavailable   = errors.New("authorization: service unavailable")
	ErrOperationUnsupported = errors.New("authorization: operation not implemented yet")
)

// Service 定义授权域的核心能力接口。
type Service interface {
	CreateCapability(ctx context.Context, req CapabilityCreateRequest) (*eventfabricmodel.AuthorizationCapability, error)
	UpdateCapability(ctx context.Context, id uuid.UUID, req CapabilityUpdateRequest) (*eventfabricmodel.AuthorizationCapability, error)
	ListCapabilities(ctx context.Context, filter CapabilityFilter) ([]*eventfabricmodel.AuthorizationCapability, error)

	CreateGrant(ctx context.Context, req GrantCreateRequest) (*GrantResult, error)
	UpdateGrant(ctx context.Context, grantID uuid.UUID, req GrantUpdateRequest) (*GrantResult, error)
	RevokeGrant(ctx context.Context, grantID uuid.UUID, actor uuid.UUID, reason string) error
	GetGrant(ctx context.Context, grantID uuid.UUID, withRelations bool) (*GrantDetail, error)
	ListGrants(ctx context.Context, filter GrantFilter) ([]*GrantSummary, int64, error)

	DecideChallenge(ctx context.Context, ticketID uuid.UUID, decision ChallengeDecisionInput) (*ChallengeDecisionResult, error)
	ProcessExpiredChallenges(ctx context.Context, tenantID uuid.UUID, before time.Time) (int, error)

	InvalidateGrantCache(ctx context.Context, key GrantCacheKey) error
	ListenCacheInvalidation(ctx context.Context) error
}

// CapabilityCreateRequest 描述能力创建参数。
type CapabilityCreateRequest struct {
	Namespace        string
	Action           string
	Description      string
	RiskLevel        string
	DefaultRateLimit map[string]any
	Metadata         map[string]any
}

// CapabilityUpdateRequest 描述能力更新参数。
type CapabilityUpdateRequest struct {
	Description      *string
	RiskLevel        *string
	DefaultRateLimit map[string]any
	Metadata         map[string]any
}

// CapabilityFilter 描述能力查询过滤项。
type CapabilityFilter struct {
	RiskLevels []string
}

// GrantCreateRequest 描述 Grant 创建参数（后续阶段填充业务字段）。
type GrantCreateRequest struct {
	// TODO: Phase 3 将补充详细字段。
}

// GrantUpdateRequest 描述 Grant 更新参数。
type GrantUpdateRequest struct {
	// TODO: Phase 3 将补充详细字段。
}

// GrantResult 提供 Grant 写入后的结果。
type GrantResult struct {
	Grant        *eventfabricmodel.AuthorizationGrant
	Capabilities []*eventfabricmodel.AuthorizationGrantCapability
	Conditions   []*eventfabricmodel.AuthorizationGrantCondition
	Challenged   bool
	Ticket       *eventfabricmodel.AuthorizationApprovalTicket
}

// GrantFilter 描述 Grant 查询过滤项。
type GrantFilter struct {
	TenantID    uuid.UUID
	Status      []string
	SubjectType string
	SubjectID   uuid.UUID
	Page        int
	PageSize    int
}

// GrantSummary 用于列表展示。
type GrantSummary struct {
	Grant *eventfabricmodel.AuthorizationGrant
}

// GrantDetail 用于详情展示。
type GrantDetail struct {
	Grant        *eventfabricmodel.AuthorizationGrant
	Capabilities []*eventfabricmodel.AuthorizationGrantCapability
	Conditions   []*eventfabricmodel.AuthorizationGrantCondition
	Ticket       *eventfabricmodel.AuthorizationApprovalTicket
}

// ChallengeDecisionInput 描述 Challenge 决策入参。
type ChallengeDecisionInput struct {
	ActorID uuid.UUID
	Result  string // approve / reject
	Reason  string
}

// ChallengeDecisionResult 返回 Challenge 决策结果。
type ChallengeDecisionResult struct {
	Ticket *eventfabricmodel.AuthorizationApprovalTicket
}

// ServiceOptions 构造 serviceImpl 所需依赖。
type ServiceOptions struct {
	Repository   *eventfabricrepo.AuthorizationRepository
	Cache        Cache
	Dispatcher   ChallengeDispatcher
	Secrets      *SecretsManager
	ChallengeSLA time.Duration
	Audit        eventaudit.Service
	Metrics      eventmetrics.Recorder
	Logger       *pxlog.Logger
	Clock        func() time.Time
}

type serviceImpl struct {
	repo         *eventfabricrepo.AuthorizationRepository
	cache        Cache
	dispatcher   ChallengeDispatcher
	secrets      *SecretsManager
	challengeSLA time.Duration
	audit        eventaudit.Service
	metrics      eventmetrics.Recorder
	logger       *pxlog.Logger
	clock        func() time.Time
}

// NewService 构建授权领域服务。
func NewService(opts ServiceOptions) (Service, error) {
	if opts.Repository == nil {
		return nil, fmt.Errorf("authorization service requires repository")
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	challengeSLA := opts.ChallengeSLA
	if challengeSLA <= 0 {
		challengeSLA = 15 * time.Minute
	}
	return &serviceImpl{
		repo:         opts.Repository,
		cache:        opts.Cache,
		dispatcher:   opts.Dispatcher,
		secrets:      opts.Secrets,
		challengeSLA: challengeSLA,
		audit:        opts.Audit,
		metrics:      opts.Metrics,
		logger:       logger,
		clock:        clock,
	}, nil
}

// ------------------ Capability APIs ------------------

func (s *serviceImpl) CreateCapability(ctx context.Context, req CapabilityCreateRequest) (*eventfabricmodel.AuthorizationCapability, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	namespace := strings.TrimSpace(req.Namespace)
	action := strings.TrimSpace(req.Action)
	if namespace == "" || action == "" {
		return nil, fmt.Errorf("namespace and action are required")
	}

	existing, err := s.repo.GetCapabilityByNamespaceAction(ctx, namespace, action)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCapabilityExists
	}

	model := &eventfabricmodel.AuthorizationCapability{
		Namespace:   namespace,
		Action:      action,
		Description: req.Description,
		RiskLevel:   normalizeRiskLevel(req.RiskLevel),
	}
	if req.DefaultRateLimit != nil {
		blob, err := json.Marshal(req.DefaultRateLimit)
		if err != nil {
			return nil, fmt.Errorf("marshal default rate limit: %w", err)
		}
		model.DefaultRateLimit = datatypes.JSON(blob)
	}
	if req.Metadata != nil {
		blob, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		model.Metadata = datatypes.JSON(blob)
	}

	created, err := s.repo.CreateCapability(ctx, model)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *serviceImpl) UpdateCapability(ctx context.Context, id uuid.UUID, req CapabilityUpdateRequest) (*eventfabricmodel.AuthorizationCapability, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if id == uuid.Nil {
		return nil, fmt.Errorf("capability id is required")
	}
	record, err := s.repo.GetCapabilityByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrCapabilityNotFound
	}

	if req.Description != nil {
		record.Description = *req.Description
	}
	if req.RiskLevel != nil {
		record.RiskLevel = normalizeRiskLevel(*req.RiskLevel)
	}
	if req.DefaultRateLimit != nil {
		blob, err := json.Marshal(req.DefaultRateLimit)
		if err != nil {
			return nil, fmt.Errorf("marshal default rate limit: %w", err)
		}
		record.DefaultRateLimit = datatypes.JSON(blob)
	}
	if req.Metadata != nil {
		blob, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		record.Metadata = datatypes.JSON(blob)
	}

	updated, err := s.repo.UpdateCapability(ctx, record)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *serviceImpl) ListCapabilities(ctx context.Context, filter CapabilityFilter) ([]*eventfabricmodel.AuthorizationCapability, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	return s.repo.ListCapabilities(ctx, filter.RiskLevels)
}

// ------------------ Grant APIs (to be implemented in later phases) ------------------

func (s *serviceImpl) CreateGrant(ctx context.Context, req GrantCreateRequest) (*GrantResult, error) {
	return nil, ErrOperationUnsupported
}

func (s *serviceImpl) UpdateGrant(ctx context.Context, grantID uuid.UUID, req GrantUpdateRequest) (*GrantResult, error) {
	return nil, ErrOperationUnsupported
}

func (s *serviceImpl) RevokeGrant(ctx context.Context, grantID uuid.UUID, actor uuid.UUID, reason string) error {
	return ErrOperationUnsupported
}

func (s *serviceImpl) GetGrant(ctx context.Context, grantID uuid.UUID, withRelations bool) (*GrantDetail, error) {
	return nil, ErrOperationUnsupported
}

func (s *serviceImpl) ListGrants(ctx context.Context, filter GrantFilter) ([]*GrantSummary, int64, error) {
	return nil, 0, ErrOperationUnsupported
}

func (s *serviceImpl) DecideChallenge(ctx context.Context, ticketID uuid.UUID, decision ChallengeDecisionInput) (*ChallengeDecisionResult, error) {
	return nil, ErrOperationUnsupported
}

func (s *serviceImpl) ProcessExpiredChallenges(ctx context.Context, tenantID uuid.UUID, before time.Time) (int, error) {
	return 0, ErrOperationUnsupported
}

// ------------------ Cache 控制 ------------------

func (s *serviceImpl) InvalidateGrantCache(ctx context.Context, key GrantCacheKey) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Invalidate(ctx, key)
}

func (s *serviceImpl) ListenCacheInvalidation(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.ListenInvalidations(ctx)
}

// ------------------ helpers ------------------

func (s *serviceImpl) ensureReady() error {
	if s == nil || s.repo == nil {
		return ErrServiceUnavailable
	}
	return nil
}

func normalizeRiskLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", eventfabricmodel.RiskLevelLow:
		return eventfabricmodel.RiskLevelLow
	case eventfabricmodel.RiskLevelMedium:
		return eventfabricmodel.RiskLevelMedium
	case eventfabricmodel.RiskLevelHigh:
		return eventfabricmodel.RiskLevelHigh
	case eventfabricmodel.RiskLevelCritical:
		return eventfabricmodel.RiskLevelCritical
	default:
		return eventfabricmodel.RiskLevelLow
	}
}

// ensure interface compliance
var _ Service = (*serviceImpl)(nil)
