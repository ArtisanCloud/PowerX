package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentProfileReader exposes only low-risk Agent governance metadata. Prompt
// seeds, personas, bindings, credentials, and runtime configuration remain
// outside the observation plane.
type AgentProfileReader struct {
	db *gorm.DB
}

func NewAgentProfileReader(db *gorm.DB) AgentProfileReader {
	return AgentProfileReader{db: db}
}

func (r AgentProfileReader) ReadObservation(ctx context.Context, resource ResourceDescriptor, purpose string) (ResourceObservation, error) {
	if r.db == nil || resource.Kind != ResourceKindAgentProfile || resource.ResourceUUID == uuid.Nil || strings.TrimSpace(resource.TenantUUID) == "" || strings.TrimSpace(purpose) == "" {
		return ResourceObservation{}, fmt.Errorf("agent profile observation is invalid")
	}
	if !resource.ReadGranted {
		return ResourceObservation{}, ErrResourceReadDenied
	}
	env := strings.TrimSpace(contextString(ctx, "env"))
	if env == "" {
		return ResourceObservation{}, fmt.Errorf("agent profile observation env is required")
	}
	var agent dbmodel.Agent
	if err := r.db.WithContext(ctx).Where("uuid = ? AND env = ? AND tenant_uuid = ?", resource.ResourceUUID, env, resource.TenantUUID).First(&agent).Error; err != nil {
		return ResourceObservation{}, fmt.Errorf("load agent profile observation: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"resource_uuid": resource.ResourceUUID.String(),
		"kind":          ResourceKindAgentProfile,
		"name":          agent.Name,
		"description":   agent.Description,
		"source":        agent.Source,
		"type_id":       agent.TypeID,
		"scene":         agent.Scene,
		"status":        agent.Status,
		"purpose":       strings.TrimSpace(purpose),
	})
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("encode agent profile observation: %w", err)
	}
	return ResourceObservation{ResourceUUID: resource.ResourceUUID, Summary: string(payload)}, nil
}
