package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CapabilityApprovalService struct {
	repo *repository.CapabilityApprovalRepository
}

type capabilityApprovalServiceContextKey struct{}

type CapabilityApprovalScope struct {
	Env              string
	TenantUUID       string
	RunUUID          uuid.UUID
	SnapshotUUID     uuid.UUID
	PlanRevisionUUID uuid.UUID
	AgentUUID        uuid.UUID
	SessionUUID      uuid.UUID
	MessageUUID      uuid.UUID
	SubjectUUID      string
}

type CapabilityApprovalSummary struct {
	ApprovalUUID uuid.UUID `json:"approval_uuid"`
	Status       string    `json:"status"`
	CreatedAt    string    `json:"created_at"`
}

// ApprovedCapabilityResume is a server-resolved continuation grant.  It is
// deliberately assembled from the persisted approval row so an HTTP caller
// cannot choose a different run, snapshot, revision, or original message.
type ApprovedCapabilityResume struct {
	Approval CapabilityApproval
	Scope    CapabilityApprovalScope
}

func (s CapabilityApprovalScope) validate() error {
	if strings.TrimSpace(s.Env) == "" || strings.TrimSpace(s.TenantUUID) == "" || s.RunUUID == uuid.Nil || s.SnapshotUUID == uuid.Nil || s.PlanRevisionUUID == uuid.Nil || s.AgentUUID == uuid.Nil || s.SessionUUID == uuid.Nil || s.MessageUUID == uuid.Nil || strings.TrimSpace(s.SubjectUUID) == "" {
		return fmt.Errorf("capability approval scope is incomplete")
	}
	return nil
}

func NewCapabilityApprovalService(db *gorm.DB) *CapabilityApprovalService {
	return &CapabilityApprovalService{repo: repository.NewCapabilityApprovalRepository(db)}
}

func ContextWithCapabilityApprovalService(ctx context.Context, service *CapabilityApprovalService) context.Context {
	if ctx == nil || service == nil {
		return ctx
	}
	return context.WithValue(ctx, capabilityApprovalServiceContextKey{}, service)
}

func CapabilityApprovalServiceFromContext(ctx context.Context) (*CapabilityApprovalService, bool) {
	if ctx == nil {
		return nil, false
	}
	service, ok := ctx.Value(capabilityApprovalServiceContextKey{}).(*CapabilityApprovalService)
	return service, ok && service != nil
}

func (s *CapabilityApprovalService) EnsurePending(ctx context.Context, scope CapabilityApprovalScope, capabilityUUID uuid.UUID) (CapabilityApproval, error) {
	if s == nil || s.repo == nil || scope.validate() != nil || capabilityUUID == uuid.Nil {
		return CapabilityApproval{}, fmt.Errorf("capability approval request is incomplete")
	}
	approvalUUID := uuid.New()
	row, err := s.repo.CreateOrGetPending(ctx, &dbmodel.AgentCapabilityApproval{
		UUID: approvalUUID,
		Env:  scope.Env, TenantUUID: scope.TenantUUID, RunUUID: scope.RunUUID, SnapshotUUID: scope.SnapshotUUID, PlanRevisionUUID: scope.PlanRevisionUUID, AgentUUID: scope.AgentUUID, SessionUUID: scope.SessionUUID, MessageUUID: scope.MessageUUID,
		CapabilityUUID: capabilityUUID, RequestedByUserUUID: scope.SubjectUUID, Status: dbmodel.AgentCapabilityApprovalPending,
		EvidenceRef: "agent_capability_approval/" + approvalUUID.String(),
	})
	if err != nil {
		return CapabilityApproval{}, err
	}
	return capabilityApprovalFromModel(row), nil
}

// ResolveApproved turns caller-supplied approval UUIDs into trusted runtime
// evidence only after every session, actor, tenant and status predicate holds.
func (s *CapabilityApprovalService) ResolveApproved(ctx context.Context, scope CapabilityApprovalScope, approvalUUIDs []uuid.UUID) ([]CapabilityApproval, error) {
	if s == nil || s.repo == nil || scope.validate() != nil {
		return nil, fmt.Errorf("capability approval resolver is not configured")
	}
	approvals := make([]CapabilityApproval, 0, len(approvalUUIDs))
	seen := make(map[uuid.UUID]struct{}, len(approvalUUIDs))
	for _, approvalUUID := range approvalUUIDs {
		if approvalUUID == uuid.Nil {
			return nil, fmt.Errorf("capability approval UUID is required")
		}
		if _, exists := seen[approvalUUID]; exists {
			return nil, fmt.Errorf("duplicate capability approval UUID")
		}
		seen[approvalUUID] = struct{}{}
		row, err := s.repo.FindScoped(ctx, scope.Env, scope.TenantUUID, scope.SubjectUUID, scope.AgentUUID, scope.SessionUUID, approvalUUID)
		if err != nil {
			return nil, fmt.Errorf("resolve capability approval: %w", err)
		}
		if row.Status != dbmodel.AgentCapabilityApprovalApproved || row.ApprovedByUserUUID == nil || strings.TrimSpace(*row.ApprovedByUserUUID) == "" || row.ApprovedAt == nil || strings.TrimSpace(row.EvidenceRef) == "" {
			return nil, fmt.Errorf("capability approval is not approved")
		}
		approvals = append(approvals, capabilityApprovalFromModel(row))
	}
	return approvals, nil
}

// LoadApprovedForResume returns exactly one approved continuation owned by the
// authenticated original requester. It never accepts run-scoping identifiers
// from the caller.
func (s *CapabilityApprovalService) LoadApprovedForResume(ctx context.Context, env, tenantUUID, subjectUUID string, agentUUID, sessionUUID, approvalUUID uuid.UUID) (*ApprovedCapabilityResume, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || strings.TrimSpace(subjectUUID) == "" || agentUUID == uuid.Nil || sessionUUID == uuid.Nil || approvalUUID == uuid.Nil {
		return nil, fmt.Errorf("approved capability resume input is incomplete")
	}
	row, err := s.repo.FindScoped(ctx, env, tenantUUID, subjectUUID, agentUUID, sessionUUID, approvalUUID)
	if err != nil {
		return nil, fmt.Errorf("find approved capability resume: %w", err)
	}
	if row.Status != dbmodel.AgentCapabilityApprovalApproved || row.ApprovedByUserUUID == nil || strings.TrimSpace(*row.ApprovedByUserUUID) == "" || row.ApprovedAt == nil || strings.TrimSpace(row.EvidenceRef) == "" {
		return nil, fmt.Errorf("capability approval is not approved")
	}
	scope := CapabilityApprovalScope{Env: row.Env, TenantUUID: row.TenantUUID, RunUUID: row.RunUUID, SnapshotUUID: row.SnapshotUUID, PlanRevisionUUID: row.PlanRevisionUUID, AgentUUID: row.AgentUUID, SessionUUID: row.SessionUUID, MessageUUID: row.MessageUUID, SubjectUUID: row.RequestedByUserUUID}
	if err := scope.validate(); err != nil {
		return nil, fmt.Errorf("persisted capability approval scope is invalid: %w", err)
	}
	return &ApprovedCapabilityResume{Approval: capabilityApprovalFromModel(row), Scope: scope}, nil
}

func (s *CapabilityApprovalService) Approve(ctx context.Context, scope CapabilityApprovalScope, approvalUUID uuid.UUID, approverUUID string) error {
	if s == nil || s.repo == nil || scope.validate() != nil || approvalUUID == uuid.Nil || strings.TrimSpace(approverUUID) == "" {
		return fmt.Errorf("capability approval decision is incomplete")
	}
	if strings.TrimSpace(approverUUID) == scope.SubjectUUID {
		return fmt.Errorf("capability approval requires a distinct approver")
	}
	row, err := s.repo.FindScoped(ctx, scope.Env, scope.TenantUUID, scope.SubjectUUID, scope.AgentUUID, scope.SessionUUID, approvalUUID)
	if err != nil {
		return fmt.Errorf("find capability approval for decision: %w", err)
	}
	return s.repo.Decide(ctx, row, approverUUID, true)
}

// ApproveByTenantAdmin is the only decision API intended for an administrative
// transport. It derives every request scope from the persisted record, never
// from request body fields supplied by the approver.
func (s *CapabilityApprovalService) ApproveByTenantAdmin(ctx context.Context, env, tenantUUID string, approvalUUID uuid.UUID, approverUUID string) error {
	if s == nil || s.repo == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || approvalUUID == uuid.Nil || strings.TrimSpace(approverUUID) == "" {
		return fmt.Errorf("capability approval decision is incomplete")
	}
	row, err := s.repo.FindForDecision(ctx, env, tenantUUID, approvalUUID)
	if err != nil {
		return fmt.Errorf("find capability approval for decision: %w", err)
	}
	if strings.TrimSpace(row.RequestedByUserUUID) == strings.TrimSpace(approverUUID) {
		return fmt.Errorf("capability approval requires a distinct approver")
	}
	return s.repo.Decide(ctx, row, approverUUID, true)
}

// RejectByTenantAdmin records an explicit terminal denial using the same
// persisted tenant scope as approval. The caller cannot supply or alter the
// original run, capability, or requester identifiers.
func (s *CapabilityApprovalService) RejectByTenantAdmin(ctx context.Context, env, tenantUUID string, approvalUUID uuid.UUID, approverUUID string) error {
	if s == nil || s.repo == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || approvalUUID == uuid.Nil || strings.TrimSpace(approverUUID) == "" {
		return fmt.Errorf("capability approval decision is incomplete")
	}
	row, err := s.repo.FindForDecision(ctx, env, tenantUUID, approvalUUID)
	if err != nil {
		return err
	}
	return s.repo.Decide(ctx, row, approverUUID, false)
}

func (s *CapabilityApprovalService) ListPendingForTenantAdmin(ctx context.Context, env, tenantUUID string) ([]CapabilityApprovalSummary, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("capability approval service is not configured")
	}
	rows, err := s.repo.ListForTenant(ctx, env, tenantUUID, dbmodel.AgentCapabilityApprovalPending, 100)
	if err != nil {
		return nil, err
	}
	out := make([]CapabilityApprovalSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, CapabilityApprovalSummary{ApprovalUUID: row.UUID, Status: row.Status, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339)})
	}
	return out, nil
}

func capabilityApprovalFromModel(row *dbmodel.AgentCapabilityApproval) CapabilityApproval {
	return CapabilityApproval{ApprovalUUID: row.UUID, CapabilityUUID: row.CapabilityUUID, EvidenceRef: row.EvidenceRef}
}

func approvalScopeFromRuntime(ctx context.Context, snapshot *ResourceSnapshot) (CapabilityApprovalScope, error) {
	if snapshot == nil {
		return CapabilityApprovalScope{}, fmt.Errorf("runtime snapshot is required for capability approval")
	}
	sessionUUID, err := uuid.Parse(strings.TrimSpace(anyToString(ctx.Value("session_uuid"))))
	if err != nil || sessionUUID == uuid.Nil {
		return CapabilityApprovalScope{}, fmt.Errorf("runtime session_uuid is required for capability approval")
	}
	messageUUID, err := uuid.Parse(strings.TrimSpace(anyToString(ctx.Value("message_uuid"))))
	if err != nil || messageUUID == uuid.Nil {
		return CapabilityApprovalScope{}, fmt.Errorf("runtime message_uuid is required for capability approval")
	}
	subjectUUID := strings.TrimSpace(reqctx.GetUserUUID(ctx))
	if subjectUUID == "" {
		return CapabilityApprovalScope{}, fmt.Errorf("runtime subject_uuid is required for capability approval")
	}
	runUUID, runErr := uuid.Parse(contextString(ctx, "runtime_run_uuid"))
	snapshotUUID, snapshotErr := uuid.Parse(contextString(ctx, "runtime_snapshot_uuid"))
	revisionUUID, revisionErr := uuid.Parse(contextString(ctx, "runtime_plan_revision_uuid"))
	if runErr != nil || snapshotErr != nil || revisionErr != nil || runUUID == uuid.Nil || snapshotUUID == uuid.Nil || revisionUUID == uuid.Nil {
		return CapabilityApprovalScope{}, fmt.Errorf("runtime approval resume references are required")
	}
	return CapabilityApprovalScope{Env: strings.TrimSpace(anyToString(ctx.Value("env"))), TenantUUID: snapshot.TenantUUID, RunUUID: runUUID, SnapshotUUID: snapshotUUID, PlanRevisionUUID: revisionUUID, AgentUUID: snapshot.AgentUUID, SessionUUID: sessionUUID, MessageUUID: messageUUID, SubjectUUID: subjectUUID}, nil
}
