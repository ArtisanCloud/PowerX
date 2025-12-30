package provider_registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/datatypes"
)

const (
	auditOpRolloutPlanned  = "provider.rollout_planned"
	auditOpRolloutRollback = "provider.rollout_rollback"
	minRollbackWindow      = 5 * time.Minute
)

// RolloutPlanInput carries gray rollout parameters for tenant-scoped deployments.
type RolloutPlanInput struct {
	Env              string
	Strategy         string
	Percentage       int
	Tenants          []TenantRef
	Note             string
	ExpiresInMinutes uint32
	RequestedBy      string
}

// RollbackInput carries metadata for rollback commands.
type RollbackInput struct {
	Env    string
	Reason string
}

// ScheduleRollout applies a gray rollout plan with tenant whitelist and percentage metadata.
func (s *Service) ScheduleRollout(ctx context.Context, providerID uuid.UUID, input RolloutPlanInput) (*model.ProviderProfile, error) {
	if len(input.Tenants) == 0 {
		return nil, errors.New("tenants required for rollout")
	}
	if input.Percentage < 0 || input.Percentage > 100 {
		return nil, errors.New("percentage must be between 0 and 100")
	}
	profile, err := s.mustProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if input.Env != "" && !strings.EqualFold(profile.Env, strings.TrimSpace(input.Env)) {
		return nil, fmt.Errorf("provider env mismatch: %s", profile.Env)
	}

	meta := utils.CloneJSONMap(profile.Metadata)
	plan := cloneJSONMap(meta["rollout_plan"])
	if plan == nil {
		plan = datatypes.JSONMap{}
	}
	now := s.clock().UTC()
	if _, exists := plan["started_at"]; !exists {
		plan["started_at"] = now.Format(time.RFC3339)
	}
	plan["updated_at"] = now.Format(time.RFC3339)
	if input.RequestedBy != "" {
		plan["requested_by"] = strings.TrimSpace(input.RequestedBy)
	}
	plan["strategy"] = sanitizeStrategy(input.Strategy)
	plan["percentage"] = input.Percentage
	plan["tenants"] = tenantRefsToAny(input.Tenants)
	plan["note"] = strings.TrimSpace(input.Note)
	plan["status"] = "gray"
	plan["rollback_deadline"] = now.Add(minRollbackWindow).Format(time.RFC3339)
	if input.ExpiresInMinutes > 0 {
		plan["expires_in_minutes"] = input.ExpiresInMinutes
	}
	if _, exists := plan["previous_whitelist"]; !exists {
		prev := DecodeTenantWhitelist(profile.TenantWhitelist)
		plan["previous_whitelist"] = tenantRefsToAny(prev)
	}

	rawWhitelist, err := marshalTenantWhitelist(input.Tenants)
	if err != nil {
		return nil, err
	}

	meta["rollout_plan"] = plan
	meta["rollout_strategy"] = plan["strategy"]
	meta["rollout_percentage"] = input.Percentage

	updates := map[string]any{
		"metadata":         meta,
		"rollout_status":   "gray",
		"tenant_whitelist": rawWhitelist,
	}
	if err := s.repo.UpdateFields(ctx, providerID, updates); err != nil {
		return nil, err
	}

	profile.Metadata = meta
	profile.RolloutStatus = "gray"
	profile.TenantWhitelist = rawWhitelist

	s.emitAudit(ctx, auditOpRolloutPlanned, profile, map[string]any{
		"percentage": input.Percentage,
		"tenants":    tenantRefsToAny(input.Tenants),
		"note":       strings.TrimSpace(input.Note),
	})
	return profile, nil
}

// RollbackProvider reverts tenant whitelist and rollout metadata.
func (s *Service) RollbackProvider(ctx context.Context, providerID uuid.UUID, input RollbackInput) (*model.ProviderProfile, error) {
	profile, err := s.mustProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if input.Env != "" && !strings.EqualFold(profile.Env, strings.TrimSpace(input.Env)) {
		return nil, fmt.Errorf("provider env mismatch: %s", profile.Env)
	}

	meta := utils.CloneJSONMap(profile.Metadata)
	plan := cloneJSONMap(meta["rollout_plan"])
	if plan == nil {
		plan = datatypes.JSONMap{}
	}
	now := s.clock().UTC()
	plan["status"] = "rolled_back"
	plan["rolled_back_at"] = now.Format(time.RFC3339)
	if strings.TrimSpace(input.Reason) != "" {
		plan["rollback_reason"] = strings.TrimSpace(input.Reason)
	}

	previous := parseTenantRefs(plan["previous_whitelist"])
	var whitelist datatypes.JSON
	if len(previous) > 0 {
		raw, err := marshalTenantWhitelist(previous)
		if err != nil {
			return nil, err
		}
		whitelist = raw
	} else {
		whitelist = profile.TenantWhitelist
	}

	meta["rollout_plan"] = plan

	updates := map[string]any{
		"metadata":       meta,
		"rollout_status": "rolled_back",
	}
	if len(previous) > 0 {
		updates["tenant_whitelist"] = whitelist
	}
	if err := s.repo.UpdateFields(ctx, providerID, updates); err != nil {
		return nil, err
	}

	profile.Metadata = meta
	profile.RolloutStatus = "rolled_back"
	if len(previous) > 0 {
		profile.TenantWhitelist = whitelist
	}

	s.emitAudit(ctx, auditOpRolloutRollback, profile, map[string]any{
		"reason": strings.TrimSpace(input.Reason),
	})
	return profile, nil
}

func cloneJSONMap(raw any) datatypes.JSONMap {
	src, ok := raw.(map[string]any)
	if !ok || src == nil {
		return nil
	}
	cloned := datatypes.JSONMap{}
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}

func tenantRefsToAny(refs []TenantRef) []map[string]string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, map[string]string{
			"tenant_uuid": r.TenantUUID,
			"environment": r.Environment,
		})
	}
	return out
}

func parseTenantRefs(raw any) []TenantRef {
	var refs []TenantRef
	switch val := raw.(type) {
	case []TenantRef:
		return val
	case []interface{}:
		for _, item := range val {
			if m, ok := item.(map[string]any); ok {
				tenant := strings.TrimSpace(getString(m["tenant_uuid"]))
				if tenant == "" {
					tenant = strings.TrimSpace(getString(m["tenantUuid"]))
				}
				refs = append(refs, TenantRef{
					TenantUUID:  tenant,
					Environment: strings.TrimSpace(getString(m["environment"])),
				})
			}
		}
	case []map[string]string:
		for _, m := range val {
			tenant := strings.TrimSpace(m["tenant_uuid"])
			if tenant == "" {
				tenant = strings.TrimSpace(m["tenantUuid"])
			}
			refs = append(refs, TenantRef{
				TenantUUID:  tenant,
				Environment: strings.TrimSpace(m["environment"]),
			})
		}
	}
	out := make([]TenantRef, 0, len(refs))
	for _, ref := range refs {
		if ref.TenantUUID != "" {
			out = append(out, ref)
		}
	}
	return out
}

func getString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
