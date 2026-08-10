package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrRegistrationRequestServiceNotConfigured = errors.New("registration request service not configured")
	ErrRegistrationRequestInvalid              = errors.New("registration request invalid")
	ErrRegistrationRequestNotFound             = errors.New("registration request not found")
	ErrRegistrationRequestStateConflict        = errors.New("registration request state conflict")
)

type RegistrationRequestTenantCreator interface {
	CreateTenantFromRegistrationRequest(ctx context.Context, tx *gorm.DB, req *modeliam.RegistrationRequest) (string, error)
}

type RegistrationRequestService struct {
	DB      *gorm.DB
	policy  *RegistrationPolicyService
	creator RegistrationRequestTenantCreator
	now     func() time.Time
}

type RegistrationRequestServiceOption func(*RegistrationRequestService)

func NewRegistrationRequestService(db *gorm.DB, opts ...RegistrationRequestServiceOption) *RegistrationRequestService {
	s := &RegistrationRequestService{
		DB:     db,
		policy: NewRegistrationPolicyService(db),
		now:    time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithRegistrationRequestPolicy(policy *RegistrationPolicyService) RegistrationRequestServiceOption {
	return func(s *RegistrationRequestService) {
		s.policy = policy
	}
}

func WithRegistrationRequestTenantCreator(creator RegistrationRequestTenantCreator) RegistrationRequestServiceOption {
	return func(s *RegistrationRequestService) {
		s.creator = creator
	}
}

func WithRegistrationRequestClock(now func() time.Time) RegistrationRequestServiceOption {
	return func(s *RegistrationRequestService) {
		if now != nil {
			s.now = now
		}
	}
}

type RegistrationRequestSubmitInput struct {
	TenantName       string
	TenantKey        string
	OwnerEmail       string
	OwnerPhone       string
	OwnerDisplayName string
	Plan             string
	InviteCodeUUID   string
	Channel          string
	Campaign         string
	IP               string
	UserAgent        string
	TraceID          string
}

type RegistrationRequestReviewInput struct {
	RequestUUID      string
	ReviewerUserUUID string
	RejectReasonCode string
}

func (s *RegistrationRequestService) Submit(ctx context.Context, in RegistrationRequestSubmitInput) (*modeliam.RegistrationRequest, error) {
	if s == nil || s.DB == nil || s.policy == nil {
		return nil, ErrRegistrationRequestServiceNotConfigured
	}
	normalized, err := normalizeRegistrationRequestSubmitInput(in)
	if err != nil {
		return nil, err
	}
	evaluation, err := s.policy.Evaluate(ctx, RegistrationPolicyEvaluateInput{
		Email:   normalized.OwnerEmail,
		Phone:   normalized.OwnerPhone,
		Channel: normalized.Channel,
	})
	if err != nil {
		return nil, err
	}
	if !evaluation.CanSubmitRequest {
		return nil, fmt.Errorf("%w: %s", ErrRegistrationRequestInvalid, evaluation.ReasonCode)
	}
	mode := modeliam.RegistrationRequestModeWaitlist
	if evaluation.Mode == modeliam.RegistrationPolicyModeApprovalRequired {
		mode = modeliam.RegistrationRequestModeApprovalRequired
	}
	payload, err := json.Marshal(map[string]any{
		"tenant_name":        normalized.TenantName,
		"tenant_key":         normalized.TenantKey,
		"owner_email":        normalized.OwnerEmail,
		"owner_phone":        normalized.OwnerPhone,
		"owner_display_name": normalized.OwnerDisplayName,
		"plan":               normalized.Plan,
		"channel":            normalized.Channel,
		"campaign":           normalized.Campaign,
	})
	if err != nil {
		return nil, err
	}
	req := &modeliam.RegistrationRequest{
		Mode:                mode,
		Status:              modeliam.RegistrationRequestStatusSubmitted,
		TenantName:          normalized.TenantName,
		TenantKey:           normalized.TenantKey,
		OwnerEmail:          normalized.OwnerEmail,
		OwnerPhone:          normalized.OwnerPhone,
		OwnerDisplayName:    normalized.OwnerDisplayName,
		Plan:                normalized.Plan,
		Channel:             normalized.Channel,
		Campaign:            normalized.Campaign,
		InviteCodeUUID:      normalized.InviteCodeUUID,
		PolicyUUID:          evaluation.PolicyUUID,
		PolicyVersion:       evaluation.PolicyVersion,
		ApprovalPayloadJSON: datatypes.JSON(payload),
	}
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(req).Error; err != nil {
			return err
		}
		return s.writeAudit(ctx, tx, registrationRequestAuditInput{
			EventType:     modeliam.RegistrationPolicyAuditEventEvaluatePending,
			PolicyUUID:    evaluation.PolicyUUID,
			PolicyVersion: evaluation.PolicyVersion,
			Contact:       firstNonEmpty(normalized.OwnerEmail, normalized.OwnerPhone),
			RequestUUID:   req.UUID.String(),
			Decision:      modeliam.RegistrationPolicyAuditDecisionPending,
			ReasonCode:    evaluation.ReasonCode,
			IP:            normalized.IP,
			UserAgent:     normalized.UserAgent,
			TraceID:       normalized.TraceID,
		})
	}); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *RegistrationRequestService) List(ctx context.Context, status string, limit int) ([]modeliam.RegistrationRequest, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationRequestServiceNotConfigured
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.DB.WithContext(ctx).Model(&modeliam.RegistrationRequest{})
	if strings.TrimSpace(status) != "" {
		q = q.Where("status = ?", strings.TrimSpace(status))
	}
	var items []modeliam.RegistrationRequest
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *RegistrationRequestService) Approve(ctx context.Context, in RegistrationRequestReviewInput) (*modeliam.RegistrationRequest, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationRequestServiceNotConfigured
	}
	if strings.TrimSpace(in.RequestUUID) == "" || strings.TrimSpace(in.ReviewerUserUUID) == "" {
		return nil, fmt.Errorf("%w: request_uuid and reviewer_user_uuid required", ErrRegistrationRequestInvalid)
	}
	var out modeliam.RegistrationRequest
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		req, err := s.loadSubmittedRequest(ctx, tx, in.RequestUUID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		req.Status = modeliam.RegistrationRequestStatusApproved
		req.ReviewedByUserUUID = strings.TrimSpace(in.ReviewerUserUUID)
		req.ReviewedAt = &now
		if req.Mode == modeliam.RegistrationRequestModeApprovalRequired {
			if s.creator == nil {
				return ErrRegistrationRequestServiceNotConfigured
			}
			tenantUUID, err := s.creator.CreateTenantFromRegistrationRequest(ctx, tx, req)
			if err != nil {
				return err
			}
			req.CreatedTenantUUID = strings.TrimSpace(tenantUUID)
			req.Status = modeliam.RegistrationRequestStatusConverted
		}
		if err := tx.Save(req).Error; err != nil {
			return err
		}
		if err := s.writeAudit(ctx, tx, registrationRequestAuditInput{
			EventType:     modeliam.RegistrationPolicyAuditEventRequestApproved,
			PolicyUUID:    req.PolicyUUID,
			PolicyVersion: req.PolicyVersion,
			Contact:       firstNonEmpty(req.OwnerEmail, req.OwnerPhone),
			RequestUUID:   req.UUID.String(),
			TenantUUID:    req.CreatedTenantUUID,
			Decision:      modeliam.RegistrationPolicyAuditDecisionAllow,
		}); err != nil {
			return err
		}
		out = *req
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *RegistrationRequestService) Reject(ctx context.Context, in RegistrationRequestReviewInput) (*modeliam.RegistrationRequest, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationRequestServiceNotConfigured
	}
	if strings.TrimSpace(in.RequestUUID) == "" || strings.TrimSpace(in.ReviewerUserUUID) == "" || strings.TrimSpace(in.RejectReasonCode) == "" {
		return nil, fmt.Errorf("%w: request_uuid reviewer_user_uuid and reject_reason_code required", ErrRegistrationRequestInvalid)
	}
	var out modeliam.RegistrationRequest
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		req, err := s.loadSubmittedRequest(ctx, tx, in.RequestUUID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		req.Status = modeliam.RegistrationRequestStatusRejected
		req.ReviewedByUserUUID = strings.TrimSpace(in.ReviewerUserUUID)
		req.ReviewedAt = &now
		req.RejectReasonCode = strings.TrimSpace(in.RejectReasonCode)
		if err := tx.Save(req).Error; err != nil {
			return err
		}
		if err := s.writeAudit(ctx, tx, registrationRequestAuditInput{
			EventType:     modeliam.RegistrationPolicyAuditEventRequestRejected,
			PolicyUUID:    req.PolicyUUID,
			PolicyVersion: req.PolicyVersion,
			Contact:       firstNonEmpty(req.OwnerEmail, req.OwnerPhone),
			RequestUUID:   req.UUID.String(),
			Decision:      modeliam.RegistrationPolicyAuditDecisionDeny,
			ReasonCode:    req.RejectReasonCode,
		}); err != nil {
			return err
		}
		out = *req
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *RegistrationRequestService) loadSubmittedRequest(ctx context.Context, tx *gorm.DB, requestUUID string) (*modeliam.RegistrationRequest, error) {
	var req modeliam.RegistrationRequest
	if err := tx.WithContext(ctx).Where("uuid = ?", strings.TrimSpace(requestUUID)).First(&req).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationRequestNotFound
		}
		return nil, err
	}
	if req.Status != modeliam.RegistrationRequestStatusSubmitted {
		return nil, ErrRegistrationRequestStateConflict
	}
	return &req, nil
}

type registrationRequestAuditInput struct {
	EventType     string
	PolicyUUID    string
	PolicyVersion int
	Contact       string
	TenantUUID    string
	RequestUUID   string
	Decision      string
	ReasonCode    string
	IP            string
	UserAgent     string
	TraceID       string
}

func (s *RegistrationRequestService) writeAudit(ctx context.Context, tx *gorm.DB, in registrationRequestAuditInput) error {
	return tx.WithContext(ctx).Create(&modeliam.RegistrationPolicyAuditEvent{
		EventType:     in.EventType,
		PolicyUUID:    in.PolicyUUID,
		PolicyVersion: in.PolicyVersion,
		ContactHash:   hashContact(in.Contact),
		TenantUUID:    in.TenantUUID,
		RequestUUID:   in.RequestUUID,
		Decision:      in.Decision,
		ReasonCode:    in.ReasonCode,
		MatchedRules:  datatypes.JSON([]byte(`[]`)),
		IP:            in.IP,
		UserAgent:     in.UserAgent,
		TraceID:       in.TraceID,
	}).Error
}

func normalizeRegistrationRequestSubmitInput(in RegistrationRequestSubmitInput) (RegistrationRequestSubmitInput, error) {
	out := RegistrationRequestSubmitInput{
		TenantName:       strings.TrimSpace(in.TenantName),
		TenantKey:        strings.TrimSpace(in.TenantKey),
		OwnerEmail:       strings.ToLower(strings.TrimSpace(in.OwnerEmail)),
		OwnerPhone:       strings.TrimSpace(in.OwnerPhone),
		OwnerDisplayName: strings.TrimSpace(in.OwnerDisplayName),
		Plan:             strings.TrimSpace(in.Plan),
		InviteCodeUUID:   strings.TrimSpace(in.InviteCodeUUID),
		Channel:          strings.ToLower(strings.TrimSpace(in.Channel)),
		Campaign:         strings.TrimSpace(in.Campaign),
		IP:               strings.TrimSpace(in.IP),
		UserAgent:        strings.TrimSpace(in.UserAgent),
		TraceID:          strings.TrimSpace(in.TraceID),
	}
	if out.Plan == "" {
		out.Plan = modeltenant.TenantPlanFree
	}
	if out.TenantName == "" {
		return out, fmt.Errorf("%w: tenant_name required", ErrRegistrationRequestInvalid)
	}
	if out.OwnerEmail == "" && out.OwnerPhone == "" {
		return out, fmt.Errorf("%w: owner_email or owner_phone required", ErrRegistrationRequestInvalid)
	}
	return out, nil
}
