package registry

import (
	"context"
	"strings"
	"time"
)

const registryUpdatedTopic = "capability.registry.updated"

type registryUpdatedEvent struct {
	CapabilityID  string    `json:"capability_id"`
	TenantUUID    string    `json:"tenant_uuid"`
	Version       uint64    `json:"version"`
	Status        string    `json:"status"`
	ChangeKind    string    `json:"change_kind"`
	UpdatedBy     string    `json:"updated_by"`
	UpdatedAt     time.Time `json:"updated_at"`
	DisableReason string    `json:"disable_reason,omitempty"`
}

func (s *Service) publishUpdateEvent(ctx context.Context, kind string, reg Registration) {
	if s.bus == nil {
		return
	}
	tenantUUID := strings.TrimSpace(reg.TenantUUID)
	event := registryUpdatedEvent{
		CapabilityID:  reg.CapabilityID,
		TenantUUID:    tenantUUID,
		Version:       reg.Version,
		Status:        reg.Status,
		ChangeKind:    kind,
		UpdatedBy:     reg.UpdatedBy,
		UpdatedAt:     reg.UpdatedAt,
		DisableReason: reg.DisableReason,
	}
	s.bus.Publish(registryUpdatedTopic, event, ctx)
}
