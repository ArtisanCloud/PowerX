package registry

import (
	"context"
	"time"
)

const registryUpdatedTopic = "capability.registry.updated"

type registryUpdatedEvent struct {
	CapabilityID  string    `json:"capability_id"`
	TenantID      string    `json:"tenant_id"`
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
	event := registryUpdatedEvent{
		CapabilityID:  reg.CapabilityID,
		TenantID:      reg.TenantID,
		Version:       reg.Version,
		Status:        reg.Status,
		ChangeKind:    kind,
		UpdatedBy:     reg.UpdatedBy,
		UpdatedAt:     reg.UpdatedAt,
		DisableReason: reg.DisableReason,
	}
	s.bus.Publish(registryUpdatedTopic, event, ctx)
}
