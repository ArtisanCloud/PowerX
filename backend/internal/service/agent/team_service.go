package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentdbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repoagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"gorm.io/gorm"
)

var (
	ErrTeamNotFound       = errors.New("agent.team_not_found")
	ErrTeamInvalidTenant  = errors.New("agent.team_invalid_tenant")
	ErrTeamMemberNotFound = errors.New("agent.team_member_not_found")
	ErrTeamMemberRole     = errors.New("agent.team_member_role_invalid")
	ErrTeamMemberAgent    = errors.New("agent.team_member_agent_invalid")
)

type TeamService struct {
	db          *gorm.DB
	teamRepo    *repoagent.AgentTeamRepository
	memberRepo  *repoagent.AgentTeamMemberRepository
	handoffRepo *repoagent.AgentHandoffTaskRepository
	agentRepo   *agentrepo.AgentRepository
	now         func() time.Time
}

type TeamCreateInput struct {
	TenantUUID           string
	ParentAgentID        uint64
	TeamName             string
	DispatchMode         string
	DefaultFailurePolicy string
	CreatedBy            string
}

type TeamMemberUpsertInput struct {
	TeamID       uint64
	TenantUUID   string
	ChildAgentID uint64
	Role         string
	Priority     int
	Enabled      bool
}

type TeamUpdateInput struct {
	TeamID               uint64
	TenantUUID           string
	ParentAgentID        uint64
	TeamName             string
	DispatchMode         string
	DefaultFailurePolicy string
}

type HandoffRecordInput struct {
	TaskID         string
	TeamID         uint64
	TenantUUID     string
	ParentAgentID  uint64
	ChildAgentID   uint64
	SessionID      uint64
	PlanID         string
	NodeID         string
	ContextRef     string
	FailurePolicy  string
	InputPayload   any
	HandoffTraceID string
}

type HandoffFinalizeInput struct {
	TaskID        string
	Status        string
	OutputPayload any
	ErrorCode     string
	ErrorSummary  string
}

func NewTeamService(db *gorm.DB) *TeamService {
	if db == nil {
		return &TeamService{}
	}
	return &TeamService{
		db:          db,
		teamRepo:    repoagent.NewAgentTeamRepository(db),
		memberRepo:  repoagent.NewAgentTeamMemberRepository(db),
		handoffRepo: repoagent.NewAgentHandoffTaskRepository(db),
		agentRepo:   agentrepo.NewAgentRepository(db),
		now:         time.Now,
	}
}

func (s *TeamService) CreateTeam(ctx context.Context, in TeamCreateInput) (*modelagent.AgentTeam, error) {
	if s == nil || s.teamRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	rec := &modelagent.AgentTeam{
		TenantUUID:           strings.TrimSpace(in.TenantUUID),
		ParentAgentID:        in.ParentAgentID,
		TeamName:             strings.TrimSpace(in.TeamName),
		DispatchMode:         strings.TrimSpace(in.DispatchMode),
		DefaultFailurePolicy: strings.TrimSpace(in.DefaultFailurePolicy),
		Status:               modelagent.TeamStatusActive,
		CreatedBy:            strings.TrimSpace(in.CreatedBy),
	}
	rec.Normalize()
	return s.teamRepo.Create(ctx, rec)
}

func (s *TeamService) ListByParent(ctx context.Context, tenantUUID string, parentAgentID uint64, includeDisabled bool) ([]modelagent.AgentTeam, error) {
	if s == nil || s.teamRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	return s.teamRepo.ListByTenantParent(ctx, tenantUUID, parentAgentID, includeDisabled)
}

func (s *TeamService) ListByTenant(ctx context.Context, tenantUUID string, includeDisabled bool) ([]modelagent.AgentTeam, error) {
	if s == nil || s.teamRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	return s.teamRepo.ListByTenant(ctx, tenantUUID, includeDisabled)
}

func (s *TeamService) SetTeamStatus(ctx context.Context, teamID uint64, tenantUUID string, status string) error {
	if s == nil || s.teamRepo == nil {
		return gorm.ErrInvalidDB
	}
	if _, err := s.ValidateTeamTenant(ctx, teamID, tenantUUID); err != nil {
		return err
	}
	return s.teamRepo.UpdateStatus(ctx, teamID, status)
}

func (s *TeamService) UpdateTeam(ctx context.Context, in TeamUpdateInput) (*modelagent.AgentTeam, error) {
	if s == nil || s.teamRepo == nil || s.memberRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	team, err := s.ValidateTeamTenant(ctx, in.TeamID, in.TenantUUID)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	nextParent := team.ParentAgentID
	parentChanged := false
	if in.ParentAgentID > 0 && in.ParentAgentID != team.ParentAgentID {
		updates["parent_agent_id"] = in.ParentAgentID
		nextParent = in.ParentAgentID
		parentChanged = true
	}
	if strings.TrimSpace(in.TeamName) != "" {
		updates["team_name"] = strings.TrimSpace(in.TeamName)
	}
	if strings.TrimSpace(in.DispatchMode) != "" {
		updates["dispatch_mode"] = strings.TrimSpace(in.DispatchMode)
	}
	if strings.TrimSpace(in.DefaultFailurePolicy) != "" {
		updates["default_failure_policy"] = strings.TrimSpace(in.DefaultFailurePolicy)
	}

	if err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txTeamRepo := repoagent.NewAgentTeamRepository(tx)
		txMemberRepo := repoagent.NewAgentTeamMemberRepository(tx)

		if errTx := txTeamRepo.UpdateByID(ctx, in.TeamID, updates); errTx != nil {
			return errTx
		}
		if parentChanged {
			if errTx := txMemberRepo.DeleteByTeamChild(ctx, in.TeamID, nextParent); errTx != nil {
				return errTx
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.teamRepo.GetByID(ctx, in.TeamID)
}

func (s *TeamService) UpsertMember(ctx context.Context, in TeamMemberUpsertInput) (*modelagent.AgentTeamMember, error) {
	if s == nil || s.memberRepo == nil || s.agentRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	team, err := s.ValidateTeamTenant(ctx, in.TeamID, in.TenantUUID)
	if err != nil {
		return nil, err
	}
	child, err := s.agentRepo.GetByID(ctx, in.ChildAgentID)
	if err != nil {
		return nil, ErrTeamMemberAgent
	}
	if child.TenantUUID == nil || !strings.EqualFold(strings.TrimSpace(*child.TenantUUID), strings.TrimSpace(in.TenantUUID)) {
		return nil, ErrTeamMemberAgent
	}
	if strings.EqualFold(strings.TrimSpace(child.Scope), agentdbmodel.AgentScopeSystem) {
		return nil, ErrTeamMemberAgent
	}
	if strings.ToLower(strings.TrimSpace(child.Status)) != agentdbmodel.AgentStatusActive {
		return nil, ErrTeamMemberAgent
	}
	if isBuiltinOrProtected(child.Meta) {
		return nil, ErrTeamMemberAgent
	}
	if in.ChildAgentID == team.ParentAgentID {
		return nil, ErrTeamMemberAgent
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if role == "" {
		role = modelagent.DefaultChildTeamRole()
	}
	if !modelagent.IsChildTeamRole(role) {
		return nil, ErrTeamMemberRole
	}
	rec := &modelagent.AgentTeamMember{
		TeamID:       in.TeamID,
		TenantUUID:   strings.TrimSpace(in.TenantUUID),
		ChildAgentID: in.ChildAgentID,
		Role:         role,
		Priority:     in.Priority,
		Enabled:      in.Enabled,
	}
	if !in.Enabled {
		rec.Enabled = false
	}
	rec.Normalize()
	return s.memberRepo.Upsert(ctx, rec)
}

func isBuiltinOrProtected(meta map[string]any) bool {
	if len(meta) == 0 {
		return false
	}
	if v, ok := meta["builtin"]; ok {
		if b, okB := v.(bool); okB && b {
			return true
		}
	}
	if v, ok := meta["protect_from_delete"]; ok {
		if b, okB := v.(bool); okB && b {
			return true
		}
	}
	return false
}

func (s *TeamService) RemoveMember(ctx context.Context, teamID uint64, tenantUUID string, childAgentID uint64) error {
	if s == nil || s.memberRepo == nil {
		return gorm.ErrInvalidDB
	}
	if _, err := s.ValidateTeamTenant(ctx, teamID, tenantUUID); err != nil {
		return err
	}
	return s.memberRepo.DeleteByTeamChild(ctx, teamID, childAgentID)
}

func (s *TeamService) DeleteTeam(ctx context.Context, teamID uint64, tenantUUID string) error {
	if s == nil || s.teamRepo == nil || s.memberRepo == nil || s.handoffRepo == nil {
		return gorm.ErrInvalidDB
	}
	if _, err := s.ValidateTeamTenant(ctx, teamID, tenantUUID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txMemberRepo := repoagent.NewAgentTeamMemberRepository(tx)
		txHandoffRepo := repoagent.NewAgentHandoffTaskRepository(tx)
		txTeamRepo := repoagent.NewAgentTeamRepository(tx)

		if err := txMemberRepo.DeleteByTeam(ctx, teamID); err != nil {
			return err
		}
		if err := txHandoffRepo.DeleteByTeam(ctx, teamID); err != nil {
			return err
		}
		return txTeamRepo.DeleteByID(ctx, teamID)
	})
}

func (s *TeamService) ListMembers(ctx context.Context, teamID uint64, tenantUUID string) ([]modelagent.AgentTeamMember, error) {
	if s == nil || s.memberRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	if _, err := s.ValidateTeamTenant(ctx, teamID, tenantUUID); err != nil {
		return nil, err
	}
	return s.memberRepo.ListEnabledByTeam(ctx, teamID)
}

func (s *TeamService) ValidateTeamTenant(ctx context.Context, teamID uint64, tenantUUID string) (*modelagent.AgentTeam, error) {
	if s == nil || s.teamRepo == nil {
		return nil, gorm.ErrInvalidDB
	}
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repoagent.ErrAgentTeamNotFound) {
			return nil, ErrTeamNotFound
		}
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(team.TenantUUID), strings.TrimSpace(tenantUUID)) {
		return nil, ErrTeamInvalidTenant
	}
	return team, nil
}

func (s *TeamService) CreateHandoffTask(ctx context.Context, in HandoffRecordInput) error {
	if s == nil || s.handoffRepo == nil {
		return gorm.ErrInvalidDB
	}
	rec := &modelagent.AgentHandoffTask{
		TaskID:         strings.TrimSpace(in.TaskID),
		TeamID:         in.TeamID,
		TenantUUID:     strings.TrimSpace(in.TenantUUID),
		ParentAgentID:  in.ParentAgentID,
		ChildAgentID:   in.ChildAgentID,
		SessionID:      in.SessionID,
		PlanID:         strings.TrimSpace(in.PlanID),
		NodeID:         strings.TrimSpace(in.NodeID),
		ContextRef:     strings.TrimSpace(in.ContextRef),
		InputDigest:    digestJSON(in.InputPayload),
		FailurePolicy:  strings.TrimSpace(in.FailurePolicy),
		Status:         modelagent.TaskStatusQueued,
		HandoffTraceID: strings.TrimSpace(in.HandoffTraceID),
	}
	rec.Normalize()
	_, err := s.handoffRepo.UpsertByTaskID(ctx, rec)
	return err
}

func (s *TeamService) MarkHandoffRunning(ctx context.Context, taskID string, handoffTraceID string) error {
	if s == nil || s.handoffRepo == nil {
		return gorm.ErrInvalidDB
	}
	return s.handoffRepo.MarkRunning(ctx, taskID, handoffTraceID)
}

func (s *TeamService) MarkHandoffFinished(ctx context.Context, in HandoffFinalizeInput) error {
	if s == nil || s.handoffRepo == nil {
		return gorm.ErrInvalidDB
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = modelagent.TaskStatusCompleted
	}
	return s.handoffRepo.MarkFinished(ctx, in.TaskID, status, digestJSON(in.OutputPayload), strings.TrimSpace(in.ErrorCode), strings.TrimSpace(in.ErrorSummary))
}

func digestJSON(v any) string {
	if v == nil {
		return ""
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("marshal-error:%T", v)
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}
