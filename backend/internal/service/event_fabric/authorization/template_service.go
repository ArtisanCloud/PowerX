package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// TemplateService 定义 Grant 模板相关操作。
type TemplateService interface {
	Create(ctx context.Context, req TemplateCreateRequest) (*eventfabricmodel.AuthorizationGrantTemplate, error)
	Update(ctx context.Context, templateID uuid.UUID, req TemplateUpdateRequest) (*eventfabricmodel.AuthorizationGrantTemplate, error)
	Delete(ctx context.Context, templateID uuid.UUID) error
	Get(ctx context.Context, templateID uuid.UUID) (*eventfabricmodel.AuthorizationGrantTemplate, error)
	List(ctx context.Context, opts TemplateListOptions) ([]*eventfabricmodel.AuthorizationGrantTemplate, int64, error)
	Apply(ctx context.Context, req TemplateApplyRequest) (*GrantCreateRequest, error)
}

type templateServiceImpl struct {
	repo  *eventfabricrepo.AuthorizationRepository
	clock func() time.Time
}

// TemplateCreateRequest 描述模板创建参数。
type TemplateCreateRequest struct {
	Name         string
	Description  string
	Source       string
	TenantID     *uuid.UUID
	Capabilities []string
	Conditions   GrantConditionsInput
	TTLSeconds   int64
	Metadata     map[string]any
	CreatedBy    *uuid.UUID
}

// TemplateUpdateRequest 描述模板更新参数。
type TemplateUpdateRequest struct {
	Description  *string
	Capabilities *[]string
	Conditions   *GrantConditionsInput
	TTLSeconds   *int64
	Metadata     map[string]any
}

// TemplateListOptions 控制模板查询行为。
type TemplateListOptions struct {
	TenantID      *uuid.UUID
	Sources       []string
	Search        string
	IncludeGlobal bool
	Page          int
	PageSize      int
}

// TemplateApplyRequest 描述模板应用参数。
type TemplateApplyRequest struct {
	TemplateID           uuid.UUID
	TenantID             uuid.UUID
	SubjectType          string
	SubjectID            uuid.UUID
	CreatedBy            *uuid.UUID
	TTLOverride          *int64
	ConditionsOverride   *GrantConditionsInput
	CapabilitiesOverride *[]string
	Notes                map[string]any
}

// NewTemplateService 构建模板服务。
func NewTemplateService(repo *eventfabricrepo.AuthorizationRepository, clock func() time.Time) TemplateService {
	if clock == nil {
		clock = time.Now
	}
	return &templateServiceImpl{
		repo:  repo,
		clock: clock,
	}
}

func (s *templateServiceImpl) Create(ctx context.Context, req TemplateCreateRequest) (*eventfabricmodel.AuthorizationGrantTemplate, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("template name is required")
	}
	if len(req.Capabilities) == 0 {
		return nil, fmt.Errorf("template must include at least one capability")
	}
	if req.TTLSeconds < 0 {
		return nil, fmt.Errorf("ttl_seconds cannot be negative")
	}

	if err := s.validateCapabilities(ctx, req.Capabilities); err != nil {
		return nil, err
	}

	conditionsJSON, err := marshalConditionsJSON(req.Conditions)
	if err != nil {
		return nil, err
	}
	capabilitiesJSON, err := json.Marshal(req.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("marshal capabilities: %w", err)
	}
	metadataJSON, err := marshalJSON(req.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	template := &eventfabricmodel.AuthorizationGrantTemplate{
		Name:         name,
		Description:  req.Description,
		Source:       normalizeGrantSource(req.Source),
		Capabilities: datatypes.JSON(capabilitiesJSON),
		Conditions:   conditionsJSON,
		TTLSeconds:   req.TTLSeconds,
		Metadata:     metadataJSON,
	}
	if req.TenantID != nil && *req.TenantID != uuid.Nil {
		template.TenantID = req.TenantID
	}
	if req.CreatedBy != nil && *req.CreatedBy != uuid.Nil {
		template.CreatedBy = req.CreatedBy
	}

	return s.repo.CreateTemplate(ctx, template)
}

func (s *templateServiceImpl) Update(ctx context.Context, templateID uuid.UUID, req TemplateUpdateRequest) (*eventfabricmodel.AuthorizationGrantTemplate, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	if templateID == uuid.Nil {
		return nil, fmt.Errorf("template id is required")
	}

	template, err := s.repo.GetTemplateByUUID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, fmt.Errorf("template not found")
	}

	if req.Description != nil {
		template.Description = *req.Description
	}
	if req.Capabilities != nil {
		if len(*req.Capabilities) == 0 {
			return nil, fmt.Errorf("template must include at least one capability")
		}
		if err := s.validateCapabilities(ctx, *req.Capabilities); err != nil {
			return nil, err
		}
		blob, err := json.Marshal(*req.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("marshal capabilities: %w", err)
		}
		template.Capabilities = datatypes.JSON(blob)
	}
	if req.Conditions != nil {
		conditionsJSON, err := marshalConditionsJSON(*req.Conditions)
		if err != nil {
			return nil, err
		}
		template.Conditions = conditionsJSON
	}
	if req.TTLSeconds != nil {
		if *req.TTLSeconds < 0 {
			return nil, fmt.Errorf("ttl_seconds cannot be negative")
		}
		template.TTLSeconds = *req.TTLSeconds
	}
	if req.Metadata != nil {
		metadataJSON, err := marshalJSON(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		template.Metadata = metadataJSON
	}

	return s.repo.UpdateTemplate(ctx, template)
}

func (s *templateServiceImpl) Delete(ctx context.Context, templateID uuid.UUID) error {
	if err := s.ensureRepo(); err != nil {
		return err
	}
	return s.repo.DeleteTemplate(ctx, templateID)
}

func (s *templateServiceImpl) Get(ctx context.Context, templateID uuid.UUID) (*eventfabricmodel.AuthorizationGrantTemplate, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	return s.repo.GetTemplateByUUID(ctx, templateID)
}

func (s *templateServiceImpl) List(ctx context.Context, opts TemplateListOptions) ([]*eventfabricmodel.AuthorizationGrantTemplate, int64, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, 0, err
	}
	filter := eventfabricrepo.TemplateFilter{
		TenantID:      opts.TenantID,
		Sources:       opts.Sources,
		Search:        opts.Search,
		IncludeGlobal: opts.IncludeGlobal,
		Page:          opts.Page,
		PageSize:      opts.PageSize,
	}
	return s.repo.ListTemplates(ctx, filter)
}

func (s *templateServiceImpl) Apply(ctx context.Context, req TemplateApplyRequest) (*GrantCreateRequest, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	if req.TemplateID == uuid.Nil {
		return nil, fmt.Errorf("template id is required")
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}
	subjectType, err := normalizeSubjectType(req.SubjectType)
	if err != nil {
		return nil, err
	}
	if req.SubjectID == uuid.Nil {
		return nil, fmt.Errorf("subject id is required")
	}

	template, err := s.repo.GetTemplateByUUID(ctx, req.TemplateID)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, fmt.Errorf("template not found")
	}

	var capabilities []string
	if req.CapabilitiesOverride != nil {
		capabilities = append(capabilities, (*req.CapabilitiesOverride)...)
	} else {
		if err := json.Unmarshal(template.Capabilities, &capabilities); err != nil {
			return nil, fmt.Errorf("unmarshal template capabilities: %w", err)
		}
	}
	if len(capabilities) == 0 {
		return nil, fmt.Errorf("template has no capabilities")
	}

	if err := s.validateCapabilities(ctx, capabilities); err != nil {
		return nil, err
	}

	var conditionsInput GrantConditionsInput
	if req.ConditionsOverride != nil {
		conditionsInput = *req.ConditionsOverride
	} else if len(template.Conditions) > 0 {
		if err := json.Unmarshal(template.Conditions, &conditionsInput); err != nil {
			return nil, fmt.Errorf("unmarshal template conditions: %w", err)
		}
	}

	ttlSeconds := template.TTLSeconds
	if req.TTLOverride != nil {
		if *req.TTLOverride < 0 {
			return nil, fmt.Errorf("ttl_seconds override cannot be negative")
		}
		ttlSeconds = *req.TTLOverride
	}

	inputs := make([]GrantCapabilityInput, 0, len(capabilities))
	for _, cap := range capabilities {
		ns, action, err := splitCapabilityKey(cap)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, GrantCapabilityInput{
			Namespace: ns,
			Action:    action,
		})
	}

	return &GrantCreateRequest{
		TenantID:     req.TenantID,
		SubjectType:  subjectType,
		SubjectID:    req.SubjectID,
		Source:       template.Source,
		TemplateID:   &template.UUID,
		TTLSeconds:   ttlSeconds,
		Capabilities: inputs,
		Conditions:   conditionsInput,
		CreatedBy:    req.CreatedBy,
		Notes:        req.Notes,
	}, nil
}

func (s *templateServiceImpl) ensureRepo() error {
	if s == nil || s.repo == nil {
		return ErrServiceUnavailable
	}
	return nil
}

func (s *templateServiceImpl) validateCapabilities(ctx context.Context, capabilities []string) error {
	seen := make(map[string]struct{}, len(capabilities))
	for _, cap := range capabilities {
		ns, action, err := splitCapabilityKey(cap)
		if err != nil {
			return err
		}
		key := capabilityKey(ns, action)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		record, err := s.repo.GetCapabilityByNamespaceAction(ctx, ns, action)
		if err != nil {
			return err
		}
		if record == nil {
			return fmt.Errorf("%w: %s.%s", ErrCapabilityNotFound, ns, action)
		}
	}
	return nil
}

func marshalConditionsJSON(input GrantConditionsInput) (datatypes.JSON, error) {
	if len(input.Resources) == 0 && len(input.ContextTags) == 0 && input.TimeWindow == nil {
		return nil, nil
	}
	payload := map[string]any{}
	if len(input.Resources) > 0 {
		payload["resources"] = input.Resources
	}
	if len(input.ContextTags) > 0 {
		payload["context_tags"] = input.ContextTags
	}
	if input.TimeWindow != nil {
		payload["time_window"] = map[string]any{
			"start": input.TimeWindow.Start.UTC(),
			"end":   input.TimeWindow.End.UTC(),
		}
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal template conditions: %w", err)
	}
	return datatypes.JSON(blob), nil
}

func splitCapabilityKey(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid capability key %q", value)
	}
	action := parts[len(parts)-1]
	namespace := strings.Join(parts[:len(parts)-1], ".")
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(action) == "" {
		return "", "", fmt.Errorf("invalid capability key %q", value)
	}
	return namespace, action, nil
}
