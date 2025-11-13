package agent_lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	tenantFormStatusPendingApproval  = "pending_approval"
	tenantFormStatusApproved         = "approved"
	tenantFormStatusRejected         = "rejected"
	tenantFormStatusActivated        = "activated"
	tenantFormStatusActivationFailed = "activation_failed"
)

// ApprovalFlow 定义审批流接口。
type ApprovalFlow interface {
	Start(ctx context.Context) string
	Approve(ctx context.Context, ticketID string)
	Reject(ctx context.Context, ticketID string)
}

func (s *Service) SubmitTenantForm(ctx context.Context, in TenantFormInput) (*TenantForm, error) {
	if s.tenantForms == nil {
		return nil, errors.New("tenant form repository not configured")
	}
	if strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.Alias) == "" {
		return nil, fmt.Errorf("tenant_id and alias are required")
	}
	if _, err := s.profiles.GetByTenantAlias(ctx, in.TenantID, in.Alias); err == nil {
		return nil, ErrAliasConflict
	}
	conflicts, err := s.policyEngine.Evaluate(ctx, PolicyConflictInput{
		TenantID:    in.TenantID,
		Alias:       in.Alias,
		Permissions: in.Permissions,
		RateLimit:   in.RateLimit,
	})
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, NewPolicyConflictError(conflicts)
	}

	form := &agentmodel.AgentTenantForm{
		TenantID:                 in.TenantID,
		Alias:                    strings.TrimSpace(in.Alias),
		DisplayName:              defaultDisplayName(in.DisplayName, in.Alias),
		Purpose:                  in.Purpose,
		PromptTemplate:           in.PromptTemplate,
		TelemetryContractVersion: in.TelemetryContractVersion,
		ToolGrants:               encodeJSON(in.ToolGrants),
		Permissions:              encodeStrings(in.Permissions),
		RateLimit:                in.RateLimit,
		Status:                   tenantFormStatusPendingApproval,
		SandboxProfile:           in.SandboxProfile,
		Metadata:                 encodeStringMap(in.Metadata),
		RequestedBy:              in.RequestedBy,
		ConflictReasons:          encodePolicyConflicts(nil),
	}
	form.WorkflowTicketID = s.approvalFlow.Start(ctx)

	created, err := s.tenantForms.Create(ctx, form)
	if err != nil {
		return nil, err
	}
	return s.toTenantForm(created)
}

func (s *Service) ListTenantForms(ctx context.Context, tenantID string, statuses []string) ([]TenantForm, error) {
	if s.tenantForms == nil {
		return nil, errors.New("tenant form repository not configured")
	}
	forms, err := s.tenantForms.ListByTenant(ctx, tenantID, statuses)
	if err != nil {
		return nil, err
	}
	result := make([]TenantForm, 0, len(forms))
	for _, form := range forms {
		view, err := s.toTenantForm(&form)
		if err != nil {
			return nil, err
		}
		result = append(result, *view)
	}
	return result, nil
}

func (s *Service) GetTenantForm(ctx context.Context, id uuid.UUID) (*TenantForm, error) {
	if s.tenantForms == nil {
		return nil, errors.New("tenant form repository not configured")
	}
	form, err := s.tenantForms.GetByUUID(ctx, id)
	if err != nil {
		return nil, ErrTenantFormNotFound
	}
	return s.toTenantForm(form)
}

func (s *Service) ApproveTenantForm(ctx context.Context, id uuid.UUID, operator string) (*TenantForm, error) {
	form, err := s.getTenantFormModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if form.Status != tenantFormStatusPendingApproval {
		return nil, ErrTenantFormInvalidStatus
	}
	s.approvalFlow.Approve(ctx, form.WorkflowTicketID)
	now := s.clock()
	form.Status = tenantFormStatusApproved
	form.ApprovedBy = operator
	form.ApprovedAt = &now
	form.LastError = ""
	if _, err := s.tenantForms.Save(ctx, form); err != nil {
		return nil, err
	}
	if err := s.activateTenantForm(ctx, form, operator); err != nil {
		return nil, err
	}
	return s.toTenantForm(form)
}

func (s *Service) RejectTenantForm(ctx context.Context, id uuid.UUID, operator, reason string) (*TenantForm, error) {
	form, err := s.getTenantFormModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if form.Status != tenantFormStatusPendingApproval {
		return nil, ErrTenantFormInvalidStatus
	}
	s.approvalFlow.Reject(ctx, form.WorkflowTicketID)
	now := s.clock()
	form.Status = tenantFormStatusRejected
	form.ApprovedBy = operator
	form.ApprovedAt = &now
	form.LastError = reason
	if _, err := s.tenantForms.Save(ctx, form); err != nil {
		return nil, err
	}
	return s.toTenantForm(form)
}

func (s *Service) getTenantFormModel(ctx context.Context, id uuid.UUID) (*agentmodel.AgentTenantForm, error) {
	if s.tenantForms == nil {
		return nil, errors.New("tenant form repository not configured")
	}
	form, err := s.tenantForms.GetByUUID(ctx, id)
	if err != nil {
		return nil, ErrTenantFormNotFound
	}
	return form, nil
}

func (s *Service) toTenantForm(model *agentmodel.AgentTenantForm) (*TenantForm, error) {
	permissions := decodeStrings(model.Permissions)
	conflicts := decodePolicyConflicts(model.ConflictReasons)
	metadata := decodeStringMap(model.Metadata)
	toolGrants := decodeToolGrantsJSON(model.ToolGrants)
	var activated *uuid.UUID
	if model.ActivatedAgentID != nil {
		activated = model.ActivatedAgentID
	}
	return &TenantForm{
		ID:                       model.UUID,
		TenantID:                 model.TenantID,
		Alias:                    model.Alias,
		DisplayName:              model.DisplayName,
		Purpose:                  model.Purpose,
		PromptTemplate:           model.PromptTemplate,
		TelemetryContractVersion: model.TelemetryContractVersion,
		ToolGrants:               toolGrants,
		Permissions:              permissions,
		RateLimit:                model.RateLimit,
		SandboxProfile:           model.SandboxProfile,
		Status:                   model.Status,
		WorkflowTicketID:         model.WorkflowTicketID,
		ConflictReasons:          conflicts,
		RequestedBy:              model.RequestedBy,
		ApprovedBy:               model.ApprovedBy,
		ApprovedAt:               model.ApprovedAt,
		ActivatedAgentID:         activated,
		Metadata:                 metadata,
		CreatedAt:                model.CreatedAt,
		UpdatedAt:                model.UpdatedAt,
	}, nil
}

func encodeJSON(v any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("null"))
	}
	bytes, _ := json.Marshal(v)
	return datatypes.JSON(bytes)
}

func encodeStrings(values []string) datatypes.JSON {
	return encodeJSON(values)
}

func decodeStrings(data datatypes.JSON) []string {
	if len(data) == 0 {
		return nil
	}
	var out []string
	_ = json.Unmarshal(data, &out)
	return out
}

func encodeStringMap(values map[string]string) datatypes.JSON {
	return encodeJSON(values)
}

func decodeStringMap(data datatypes.JSON) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func encodePolicyConflicts(values []PolicyConflict) datatypes.JSON {
	return encodeJSON(values)
}

func decodePolicyConflicts(data datatypes.JSON) []PolicyConflict {
	if len(data) == 0 {
		return nil
	}
	var out []PolicyConflict
	_ = json.Unmarshal(data, &out)
	return out
}

func decodeToolGrantsJSON(data datatypes.JSON) []ToolGrant {
	if len(data) == 0 {
		return nil
	}
	var grants []ToolGrant
	_ = json.Unmarshal(data, &grants)
	return grants
}

func newInMemoryApprovalFlow() ApprovalFlow {
	return &memoryApprovalFlow{
		tickets: make(map[string]string),
	}
}

type memoryApprovalFlow struct {
	mu      sync.Mutex
	tickets map[string]string
}

func (m *memoryApprovalFlow) Start(_ context.Context) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	m.tickets[id] = tenantFormStatusPendingApproval
	return id
}

func (m *memoryApprovalFlow) Approve(_ context.Context, ticketID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets[ticketID] = tenantFormStatusApproved
}

func (m *memoryApprovalFlow) Reject(_ context.Context, ticketID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets[ticketID] = tenantFormStatusRejected
}
