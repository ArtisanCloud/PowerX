package capability_registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
)

// WorkflowTemplateServiceOptions 配置模板服务依赖。
type WorkflowTemplateServiceOptions struct {
	TemplateRepo *repo.WorkflowTemplateRepository
	ApprovalRepo *repo.WorkflowTemplateApprovalRepository
	Clock        func() time.Time
}

// WorkflowTemplateService 聚合模板信息与审批状态。
type WorkflowTemplateService struct {
	templates *repo.WorkflowTemplateRepository
	approvals *repo.WorkflowTemplateApprovalRepository
	now       func() time.Time
}

// TemplateUpgradeInput 描述一次手动升级操作。
type TemplateUpgradeInput struct {
	TemplateID       string
	CapabilitiesHash string
	Reason           string
	Operator         string
}

// WorkflowTemplateView 组合模板与审批状态。
type WorkflowTemplateView struct {
	Template *models.WorkflowTemplateRef
	Approval *models.WorkflowTemplateApproval
}

var (
	// ErrWorkflowTemplateHashMismatch 当提供的 hash 与最新快照不一致时返回。
	ErrWorkflowTemplateHashMismatch = errors.New("workflow template capabilities_hash mismatch")
)

// NewWorkflowTemplateService 构建模板服务。
func NewWorkflowTemplateService(opts WorkflowTemplateServiceOptions) *WorkflowTemplateService {
	if opts.TemplateRepo == nil || opts.ApprovalRepo == nil {
		return nil
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &WorkflowTemplateService{
		templates: opts.TemplateRepo,
		approvals: opts.ApprovalRepo,
		now:       clock,
	}
}

// ApproveUpgrade 记录管理员对模板版本的确认。
func (s *WorkflowTemplateService) ApproveUpgrade(ctx context.Context, input TemplateUpgradeInput) (*models.WorkflowTemplateApproval, error) {
	if s == nil || s.templates == nil || s.approvals == nil {
		return nil, errors.New("workflow template service unavailable")
	}
	templateID := strings.TrimSpace(input.TemplateID)
	if templateID == "" {
		return nil, errors.New("template_id is required")
	}
	expectedHash := strings.TrimSpace(input.CapabilitiesHash)
	if expectedHash == "" {
		return nil, errors.New("capabilities_hash is required")
	}

	template, err := s.templates.GetByTemplateID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(template.CapabilitiesHash), expectedHash) {
		return nil, ErrWorkflowTemplateHashMismatch
	}

	now := s.now().UTC()
	record := &models.WorkflowTemplateApproval{
		TemplateID:       template.TemplateID,
		CapabilityID:     template.CapabilityID,
		CapabilitiesHash: expectedHash,
		Reason:           strings.TrimSpace(input.Reason),
		ApprovedBy:       strings.TrimSpace(input.Operator),
		ApprovedAt:       now,
	}
	return s.approvals.Upsert(ctx, record)
}

// GetTemplateView 返回模板及其审批状态。
func (s *WorkflowTemplateService) GetTemplateView(ctx context.Context, templateID string) (WorkflowTemplateView, error) {
	view := WorkflowTemplateView{}
	if s == nil || s.templates == nil {
		return view, errors.New("workflow template service unavailable")
	}
	template, err := s.templates.GetByTemplateID(ctx, templateID)
	if err != nil {
		return view, err
	}
	view.Template = template
	if s.approvals != nil {
		if approval, err := s.approvals.GetByTemplateID(ctx, templateID); err == nil {
			view.Approval = approval
		} else if !errors.Is(err, repo.ErrWorkflowTemplateApprovalNotFound) {
			return view, fmt.Errorf("load template approval: %w", err)
		}
	}
	return view, nil
}

// ApprovalByTemplate 返回审批记录（若存在）。
func (s *WorkflowTemplateService) ApprovalByTemplate(ctx context.Context, templateID string) (*models.WorkflowTemplateApproval, error) {
	if s == nil || s.approvals == nil {
		return nil, errors.New("workflow template service unavailable")
	}
	return s.approvals.GetByTemplateID(ctx, templateID)
}

// ListApprovals 返回所有模板的审批信息映射。
func (s *WorkflowTemplateService) ListApprovals(ctx context.Context) (map[string]*models.WorkflowTemplateApproval, error) {
	result := make(map[string]*models.WorkflowTemplateApproval)
	if s == nil || s.approvals == nil {
		return result, errors.New("workflow template service unavailable")
	}
	records, err := s.approvals.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		record := records[i]
		result[record.TemplateID] = &records[i]
	}
	return result, nil
}
