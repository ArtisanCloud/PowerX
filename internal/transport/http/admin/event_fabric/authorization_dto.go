package eventfabric

import "time"

type subjectDTO struct {
	Type string `json:"type" validate:"required,oneof=agent plugin"`
	ID   string `json:"id" validate:"required,uuid4"`
}

type grantConditionsDTO struct {
	Resources   []string       `json:"resources"`
	ContextTags []string       `json:"context_tags"`
	TimeWindow  *timeWindowDTO `json:"time_window"`
}

type timeWindowDTO struct {
	Start time.Time `json:"start" validate:"required"`
	End   time.Time `json:"end" validate:"required"`
}

type createGrantDTO struct {
	TenantID     string              `json:"tenant_id" validate:"required,uuid4"`
	Subject      subjectDTO          `json:"subject" validate:"required,dive"`
	Capabilities []string            `json:"capabilities" validate:"required,min=1,dive,required"`
	Conditions   *grantConditionsDTO `json:"conditions"`
	TTLSeconds   int64               `json:"ttl_seconds" validate:"required,min=60"`
	Source       string              `json:"source"`
	TemplateID   string              `json:"template_id"`
	Notes        map[string]any      `json:"notes"`
}

type updateGrantDTO struct {
	Capabilities *[]grantCapabilityDTO `json:"capabilities"`
	Conditions   *grantConditionsDTO   `json:"conditions"`
	TTLSeconds   *int64                `json:"ttl_seconds"`
	Reason       string                `json:"reason"`
	Notes        map[string]any        `json:"notes"`
}

type grantCapabilityDTO struct {
	Capability string         `json:"capability" validate:"required"`
	RateLimit  map[string]any `json:"rate_limit"`
}

type revokeGrantDTO struct {
	Reason string `json:"reason"`
}

type challengeDecisionDTO struct {
	Decision       string `json:"decision" validate:"required,oneof=approve reject"`
	DecisionReason string `json:"decision_reason"`
}

type invalidateCacheDTO struct {
	TenantID     string     `json:"tenant_id" validate:"required,uuid4"`
	Subject      subjectDTO `json:"subject" validate:"required,dive"`
	GrantVersion *uint64    `json:"grant_version"`
}

type createCapabilityDTO struct {
	Namespace        string         `json:"namespace" validate:"required"`
	Action           string         `json:"action" validate:"required"`
	Description      string         `json:"description"`
	RiskLevel        string         `json:"risk_level"`
	DefaultRateLimit map[string]any `json:"default_rate_limit"`
	Metadata         map[string]any `json:"metadata"`
}

type updateCapabilityDTO struct {
	Description      *string        `json:"description"`
	RiskLevel        *string        `json:"risk_level"`
	DefaultRateLimit map[string]any `json:"default_rate_limit"`
	Metadata         map[string]any `json:"metadata"`
}

type createTemplateDTO struct {
	Name         string              `json:"name" validate:"required"`
	Description  string              `json:"description"`
	Source       string              `json:"source"`
	TenantID     string              `json:"tenant_id"`
	Capabilities []string            `json:"capabilities" validate:"required,min=1,dive,required"`
	Conditions   *grantConditionsDTO `json:"conditions"`
	TTLSeconds   int64               `json:"ttl_seconds"`
	Metadata     map[string]any      `json:"metadata"`
}

type updateTemplateDTO struct {
	Description  *string             `json:"description"`
	Capabilities *[]string           `json:"capabilities"`
	Conditions   *grantConditionsDTO `json:"conditions"`
	TTLSeconds   *int64              `json:"ttl_seconds"`
	Metadata     map[string]any      `json:"metadata"`
}

type applyTemplateDTO struct {
	TenantID     string              `json:"tenant_id" validate:"required,uuid4"`
	Subject      subjectDTO          `json:"subject" validate:"required,dive"`
	TTLSeconds   *int64              `json:"ttl_seconds"`
	Conditions   *grantConditionsDTO `json:"conditions"`
	Capabilities *[]string           `json:"capabilities"`
	Notes        map[string]any      `json:"notes"`
}

type auditQueryDTO struct {
	TenantID    string `form:"tenantId" validate:"required,uuid4"`
	SubjectID   string `form:"subjectId" validate:"omitempty,uuid4"`
	SubjectType string `form:"subjectType" validate:"omitempty,oneof=agent plugin"`
	Decision    string `form:"decision" validate:"omitempty,oneof=allow block challenge"`
	Capability  string `form:"capability"`
	From        string `form:"from" validate:"required"`
	To          string `form:"to" validate:"required"`
	Page        int    `form:"page"`
	PageSize    int    `form:"pageSize"`
	Format      string `form:"format" validate:"omitempty,oneof=json csv"`
}
