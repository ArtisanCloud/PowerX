package agent_lifecycle

import (
	"encoding/json"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	ShareStatusPending = "pending"
	ShareStatusActive  = "active"
	ShareStatusRevoked = "revoked"
	ShareStatusError   = "error"
)

// AgentShare 表示对外共享记录。
type AgentShare struct {
	ID               uuid.UUID
	AgentID          uuid.UUID
	TenantID         string
	Status           string
	Quotas           []ShareQuota
	Metadata         map[string]string
	IssuedBy         string
	ValidatedAt      *string
	ProvisionedAt    *string
	NextReviewAt     *string
	CreatedAt        string
	UpdatedAt        string
	RevokedAt        *string
	RevokedBy        string
	Reason           string
	ValidationFailed bool
	ValidationError  string
}

// ShareQuota 描述配额类型。
type ShareQuota struct {
	Type  string `json:"type"`
	Limit int32  `json:"limit"`
}

func toAgentShare(model *agentmodel.AgentShareRecord) *AgentShare {
	if model == nil {
		return nil
	}
	return &AgentShare{
		ID:               model.UUID,
		AgentID:          model.AgentUUID,
		TenantID:         model.TargetTenantID,
		Status:           model.Status,
		Quotas:           decodeShareQuotas(model.Quotas),
		Metadata:         decodeStringMap(model.Metadata),
		IssuedBy:         model.IssuedBy,
		ValidatedAt:      formatPtrTime(model.ValidatedAt),
		ProvisionedAt:    formatPtrTime(model.ProvisionedAt),
		NextReviewAt:     formatPtrTime(model.NextReviewAt),
		CreatedAt:        model.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        model.UpdatedAt.UTC().Format(time.RFC3339),
		RevokedAt:        formatPtrTime(model.RevokedAt),
		RevokedBy:        model.RevokedBy,
		Reason:           model.RevokeReason,
		ValidationFailed: model.ValidationFail,
		ValidationError:  model.ValidationError,
	}
}

func decodeShareQuotas(data datatypes.JSON) []ShareQuota {
	if len(data) == 0 {
		return nil
	}
	var quotas []ShareQuota
	_ = json.Unmarshal(data, &quotas)
	return quotas
}

func encodeShareQuotas(quotas []ShareQuota) datatypes.JSON {
	return encodeJSON(quotas)
}

func formatPtrTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format(time.RFC3339)
	return &formatted
}

// ShareInput 共享请求。
type ShareInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Quotas      []ShareQuota
	Metadata    map[string]string
	RequestedBy string
	TraceID     string
}

// RevokeShareInput 撤销请求。
type RevokeShareInput struct {
	ShareID     uuid.UUID
	Reason      string
	RequestedBy string
	TraceID     string
}
