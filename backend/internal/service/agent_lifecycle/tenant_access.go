package agent_lifecycle

import (
	"context"
	"errors"
	"strings"

	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/google/uuid"
)

// EnsureTenantAccess verifies that an agent UUID belongs to the caller tenant
// before a tenant-facing transport reads or changes its lifecycle state.
func (s *Service) EnsureTenantAccess(ctx context.Context, agentID uuid.UUID, tenantUUID string) error {
	if s == nil || s.profiles == nil {
		return errors.New("agent lifecycle service not available")
	}
	if agentID == uuid.Nil {
		return errors.New("agent_id is required")
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return errors.New("tenant_uuid is required")
	}
	profile, err := s.profiles.GetByUUID(ctx, agentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return ErrAgentNotFound
		}
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(profile.TenantUUID), tenantUUID) {
		// Deliberately return not-found: tenant callers must not learn whether an
		// agent UUID exists in another tenant.
		return ErrAgentNotFound
	}
	return nil
}
