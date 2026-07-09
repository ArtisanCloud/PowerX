package agent_authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidPermissionCode  = errors.New("invalid permission_code")
	ErrCapabilityNotFound     = errors.New("capability not found")
	ErrCapabilityNotGrantable = errors.New("capability is not grantable to agent")
	ErrAgentGrantMissing      = errors.New("agent grant missing")
	ErrUserPermissionMissing  = errors.New("user permission missing")
	ErrTenantCapabilityOff    = errors.New("tenant capability disabled")
)

type Service struct {
	db     *gorm.DB
	agents *agentrepo.AgentRepository
	grants *agentrepo.AgentCapabilityGrantRepository
	rbac   *iamsvc.RBACService
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db:     db,
		agents: agentrepo.NewAgentRepository(db),
		grants: agentrepo.NewAgentCapabilityGrantRepository(db),
		rbac:   iamsvc.NewRBACService(db),
	}
}

type GrantableCapability struct {
	CapabilityUUID uuid.UUID  `json:"capability_uuid"`
	CapabilityID   string     `json:"capability_id"`
	PluginID       string     `json:"plugin_id"`
	PluginUUID     *uuid.UUID `json:"plugin_uuid,omitempty"`
	DisplayName    string     `json:"display_name"`
	Description    string     `json:"description,omitempty"`
	PermissionCode string     `json:"permission_code"`
	RiskLevel      string     `json:"risk_level"`
	AgentUsable    bool       `json:"agent_usable"`
	TenantEnabled  bool       `json:"tenant_enabled"`
	Status         string     `json:"status"`
}

type AgentGrantInput struct {
	CapabilityUUID uuid.UUID `json:"capability_uuid"`
	PermissionCode string    `json:"permission_code"`
	Enabled        bool      `json:"enabled"`
}

type AgentGrantView struct {
	UUID           uuid.UUID  `json:"uuid"`
	AgentUUID      uuid.UUID  `json:"agent_uuid"`
	CapabilityUUID uuid.UUID  `json:"capability_uuid"`
	PluginUUID     *uuid.UUID `json:"plugin_uuid,omitempty"`
	CapabilityID   string     `json:"capability_id"`
	PluginID       string     `json:"plugin_id,omitempty"`
	PermissionCode string     `json:"permission_code"`
	RiskLevel      string     `json:"risk_level"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
}

type EffectivePermissionItem struct {
	CapabilityUUID   uuid.UUID `json:"capability_uuid"`
	CapabilityID     string    `json:"capability_id"`
	PluginID         string    `json:"plugin_id"`
	DisplayName      string    `json:"display_name"`
	PermissionCode   string    `json:"permission_code"`
	RiskLevel        string    `json:"risk_level"`
	UserAllowed      bool      `json:"user_allowed"`
	AgentAllowed     bool      `json:"agent_allowed"`
	TenantEnabled    bool      `json:"tenant_enabled"`
	PolicyAllowed    bool      `json:"policy_allowed"`
	EffectiveAllowed bool      `json:"effective_allowed"`
	DenyReason       string    `json:"deny_reason,omitempty"`
}

type EffectivePermissionsResult struct {
	TenantUUID string                    `json:"tenant_uuid"`
	UserUUID   string                    `json:"user_uuid"`
	MemberUUID string                    `json:"member_uuid"`
	AgentUUID  uuid.UUID                 `json:"agent_uuid"`
	Items      []EffectivePermissionItem `json:"items"`
}

type AuthorizeInput struct {
	Env            string
	TenantUUID     string
	UserUUID       string
	MemberID       uint64
	IsRoot         bool
	AgentID        uint64
	AgentUUID      uuid.UUID
	CapabilityID   string
	PermissionCode string
}

type AuthorizeResult struct {
	Allowed        bool   `json:"allowed"`
	DenyReason     string `json:"deny_reason,omitempty"`
	PermissionCode string `json:"permission_code"`
}

func (s *Service) ListGrantableCapabilities(ctx context.Context, tenantUUID string) ([]GrantableCapability, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("agent authz service is not configured")
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil, fmt.Errorf("tenant_uuid is required")
	}
	var records []capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).
		Model(&capmodels.CapabilityRecord{}).
		Where("status IN ?", []string{"active", "published", "ready"}).
		Order("plugin_id ASC, capability_id ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]GrantableCapability, 0, len(records))
	for _, rec := range records {
		permissionCodes := permissionCodesFromRecord(rec)
		if len(permissionCodes) == 0 {
			continue
		}
		agentUsable := agentUsableFromRecord(rec)
		tenantEnabled := s.tenantCapabilityEnabled(ctx, tenantUUID, rec.CapabilityID)
		risk := riskLevelFromRecord(rec)
		for _, code := range permissionCodes {
			out = append(out, GrantableCapability{
				CapabilityUUID: rec.UUID,
				CapabilityID:   rec.CapabilityID,
				PluginID:       rec.PluginID,
				DisplayName:    rec.Title,
				Description:    rec.Description,
				PermissionCode: code,
				RiskLevel:      risk,
				AgentUsable:    agentUsable,
				TenantEnabled:  tenantEnabled,
				Status:         rec.Status,
			})
		}
	}
	return out, nil
}

func (s *Service) ListAgentGrants(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID) ([]AgentGrantView, error) {
	if err := s.requireAgent(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	rows, err := s.grants.ListByAgent(ctx, env, tenantUUID, agentUUID)
	if err != nil {
		return nil, err
	}
	return grantViews(rows), nil
}

func (s *Service) ReplaceAgentGrants(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, actorUserUUID string, inputs []AgentGrantInput) ([]AgentGrantView, error) {
	if err := s.requireAgent(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	catalog, err := s.ListGrantableCapabilities(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]GrantableCapability, len(catalog))
	for _, item := range catalog {
		byKey[grantableKey(item.CapabilityUUID, item.PermissionCode)] = item
	}
	rows := make([]agentmodel.AgentCapabilityGrant, 0, len(inputs))
	seen := map[string]struct{}{}
	for _, in := range inputs {
		code := strings.TrimSpace(in.PermissionCode)
		if in.CapabilityUUID == uuid.Nil || code == "" {
			return nil, ErrInvalidPermissionCode
		}
		item, ok := byKey[grantableKey(in.CapabilityUUID, code)]
		if !ok {
			return nil, ErrCapabilityNotFound
		}
		if !item.AgentUsable {
			return nil, ErrCapabilityNotGrantable
		}
		if !item.TenantEnabled {
			return nil, ErrTenantCapabilityOff
		}
		key := grantableKey(in.CapabilityUUID, code)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		status := agentmodel.AgentCapabilityGrantStatusDisabled
		if in.Enabled {
			status = agentmodel.AgentCapabilityGrantStatusEnabled
		}
		rows = append(rows, agentmodel.AgentCapabilityGrant{
			AgentUUID:         agentUUID,
			CapabilityUUID:    item.CapabilityUUID,
			PluginUUID:        item.PluginUUID,
			CapabilityID:      item.CapabilityID,
			PluginID:          item.PluginID,
			PermissionCode:    code,
			RiskLevel:         firstNonEmpty(item.RiskLevel, "unknown"),
			Status:            status,
			Source:            agentmodel.AgentCapabilityGrantSourceManual,
			CreatedByUserUUID: strings.TrimSpace(actorUserUUID),
			UpdatedByUserUUID: strings.TrimSpace(actorUserUUID),
		})
	}
	if err := s.grants.ReplaceByAgent(ctx, env, tenantUUID, agentUUID, rows); err != nil {
		return nil, err
	}
	return s.ListAgentGrants(ctx, env, tenantUUID, agentUUID)
}

func (s *Service) ResolveEffectivePermissions(ctx context.Context, env, tenantUUID, userUUID, memberUUID string, memberID uint64, isRoot bool, agentUUID uuid.UUID) (EffectivePermissionsResult, error) {
	if err := s.requireAgent(ctx, env, tenantUUID, agentUUID); err != nil {
		return EffectivePermissionsResult{}, err
	}
	catalog, err := s.ListGrantableCapabilities(ctx, tenantUUID)
	if err != nil {
		return EffectivePermissionsResult{}, err
	}
	grants, err := s.grants.ListByAgent(ctx, env, tenantUUID, agentUUID)
	if err != nil {
		return EffectivePermissionsResult{}, err
	}
	agentAllowed := map[string]bool{}
	for _, grant := range grants {
		if strings.EqualFold(grant.Status, agentmodel.AgentCapabilityGrantStatusEnabled) {
			agentAllowed[grantableKey(grant.CapabilityUUID, grant.PermissionCode)] = true
		}
	}
	items := make([]EffectivePermissionItem, 0, len(catalog))
	for _, cap := range catalog {
		module, resource, action, parseOK := ParsePermissionCode(cap.PermissionCode)
		userAllowed := false
		if parseOK {
			userAllowed, err = s.rbac.Enforce(ctx, iamsvc.ActorContext{IsRoot: isRoot, TenantUUID: tenantUUID}, tenantUUID, memberID, module, resource, action)
			if err != nil {
				return EffectivePermissionsResult{}, err
			}
		}
		agAllowed := agentAllowed[grantableKey(cap.CapabilityUUID, cap.PermissionCode)]
		policyAllowed := cap.AgentUsable
		effective := userAllowed && agAllowed && cap.TenantEnabled && policyAllowed && parseOK
		deny := ""
		switch {
		case !parseOK:
			deny = "permission_code_invalid"
		case !userAllowed:
			deny = "user_permission_missing"
		case !agAllowed:
			deny = "agent_grant_missing"
		case !cap.TenantEnabled:
			deny = "tenant_capability_disabled"
		case !policyAllowed:
			deny = "capability_policy_denied"
		}
		items = append(items, EffectivePermissionItem{
			CapabilityUUID:   cap.CapabilityUUID,
			CapabilityID:     cap.CapabilityID,
			PluginID:         cap.PluginID,
			DisplayName:      cap.DisplayName,
			PermissionCode:   cap.PermissionCode,
			RiskLevel:        cap.RiskLevel,
			UserAllowed:      userAllowed,
			AgentAllowed:     agAllowed,
			TenantEnabled:    cap.TenantEnabled,
			PolicyAllowed:    policyAllowed,
			EffectiveAllowed: effective,
			DenyReason:       deny,
		})
	}
	return EffectivePermissionsResult{
		TenantUUID: tenantUUID,
		UserUUID:   userUUID,
		MemberUUID: memberUUID,
		AgentUUID:  agentUUID,
		Items:      items,
	}, nil
}

func (s *Service) AuthorizeCapability(ctx context.Context, in AuthorizeInput) (AuthorizeResult, error) {
	tenantUUID := strings.TrimSpace(firstNonEmpty(in.TenantUUID, reqctx.GetTenantUUID(ctx)))
	if tenantUUID == "" {
		return AuthorizeResult{Allowed: false, DenyReason: "tenant_missing"}, nil
	}
	env := firstNonEmpty(strings.TrimSpace(in.Env), reqctx.GetEnv(ctx), "dev")
	agentUUID := in.AgentUUID
	if agentUUID == uuid.Nil {
		agent, err := s.agentByNumericID(ctx, in.AgentID, tenantUUID)
		if err != nil {
			return AuthorizeResult{Allowed: false, DenyReason: "agent_missing"}, err
		}
		agentUUID = agent.UUID
	}
	capabilityID := strings.TrimSpace(in.CapabilityID)
	if capabilityID == "" {
		return AuthorizeResult{Allowed: false, DenyReason: "capability_missing"}, nil
	}
	rec, err := s.capabilityByID(ctx, capabilityID)
	if err != nil {
		return AuthorizeResult{Allowed: false, DenyReason: "capability_missing"}, err
	}
	permissionCode := strings.TrimSpace(in.PermissionCode)
	if permissionCode == "" {
		codes := permissionCodesFromRecord(*rec)
		if len(codes) == 1 {
			permissionCode = codes[0]
		}
	}
	module, resource, action, ok := ParsePermissionCode(permissionCode)
	if !ok {
		return AuthorizeResult{Allowed: false, DenyReason: "permission_code_invalid", PermissionCode: permissionCode}, nil
	}
	if !s.tenantCapabilityEnabled(ctx, tenantUUID, capabilityID) {
		return AuthorizeResult{Allowed: false, DenyReason: "tenant_capability_disabled", PermissionCode: permissionCode}, nil
	}
	hasGrant, err := s.grants.HasEnabledGrant(ctx, env, tenantUUID, agentUUID, capabilityID, permissionCode)
	if err != nil {
		return AuthorizeResult{Allowed: false, DenyReason: "agent_grant_check_failed", PermissionCode: permissionCode}, err
	}
	if !hasGrant {
		return AuthorizeResult{Allowed: false, DenyReason: "agent_grant_missing", PermissionCode: permissionCode}, nil
	}
	userAllowed, err := s.rbac.Enforce(ctx, iamsvc.ActorContext{IsRoot: in.IsRoot, TenantUUID: tenantUUID}, tenantUUID, in.MemberID, module, resource, action)
	if err != nil {
		return AuthorizeResult{Allowed: false, DenyReason: "user_permission_check_failed", PermissionCode: permissionCode}, err
	}
	if !userAllowed {
		return AuthorizeResult{Allowed: false, DenyReason: "user_permission_missing", PermissionCode: permissionCode}, nil
	}
	return AuthorizeResult{Allowed: true, PermissionCode: permissionCode}, nil
}

func (s *Service) requireAgent(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID) error {
	if strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || agentUUID == uuid.Nil {
		return fmt.Errorf("env, tenant_uuid and agent_uuid are required")
	}
	tenantRef := strings.TrimSpace(tenantUUID)
	_, err := s.agents.FindByScopeUUID(ctx, strings.TrimSpace(env), &tenantRef, agentUUID)
	return err
}

func (s *Service) agentByNumericID(ctx context.Context, agentID uint64, tenantUUID string) (*agentmodel.Agent, error) {
	if agentID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	agent, err := s.agents.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent.TenantUUID == nil || !strings.EqualFold(strings.TrimSpace(*agent.TenantUUID), strings.TrimSpace(tenantUUID)) {
		return nil, gorm.ErrRecordNotFound
	}
	return agent, nil
}

func (s *Service) capabilityByID(ctx context.Context, capabilityID string) (*capmodels.CapabilityRecord, error) {
	var rec capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).
		Where("capability_id = ?", strings.TrimSpace(capabilityID)).
		First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCapabilityNotFound
		}
		return nil, err
	}
	return &rec, nil
}

func (s *Service) tenantCapabilityEnabled(ctx context.Context, tenantUUID, capabilityID string) bool {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&capmodels.CapabilityRegistration{}).
		Where("tenant_uuid = ? AND capability_id = ? AND status IN ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(capabilityID), []string{"active", "published", "ready", "enabled"}).
		Count(&count).Error
	return err == nil && count > 0
}

func permissionCodesFromRecord(rec capmodels.CapabilityRecord) []string {
	var annotations map[string]any
	_ = json.Unmarshal(rec.Annotations, &annotations)
	candidates := stringListFromAny(annotations["permission_codes"])
	if code := strings.TrimSpace(anyString(annotations["permission_code"])); code != "" {
		candidates = append(candidates, code)
	}
	if len(candidates) == 0 {
		candidates = stringListFromJSON(rec.ToolScope)
	}
	out := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, code := range candidates {
		code = strings.TrimSpace(code)
		if _, _, _, ok := ParsePermissionCode(code); !ok {
			continue
		}
		key := strings.ToLower(code)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func agentUsableFromRecord(rec capmodels.CapabilityRecord) bool {
	var annotations map[string]any
	if err := json.Unmarshal(rec.Annotations, &annotations); err == nil {
		if v, ok := annotations["agent_usable"]; ok {
			return boolFromAny(v)
		}
	}
	return true
}

func riskLevelFromRecord(rec capmodels.CapabilityRecord) string {
	var annotations map[string]any
	if err := json.Unmarshal(rec.Annotations, &annotations); err == nil {
		if v := strings.TrimSpace(anyString(annotations["risk_level"])); v != "" {
			return v
		}
	}
	return "unknown"
}

func ParsePermissionCode(code string) (module, resource, action string, ok bool) {
	code = strings.TrimSpace(code)
	left, right, found := strings.Cut(code, ":")
	if !found {
		return "", "", "", false
	}
	action = strings.TrimSpace(right)
	parts := strings.Split(strings.TrimSpace(left), ".")
	if len(parts) < 2 || action == "" {
		return "", "", "", false
	}
	module = strings.TrimSpace(parts[0])
	resource = strings.TrimSpace(strings.Join(parts[1:], "."))
	if module == "" || resource == "" {
		return "", "", "", false
	}
	return module, resource, action, true
}

func grantViews(rows []agentmodel.AgentCapabilityGrant) []AgentGrantView {
	out := make([]AgentGrantView, 0, len(rows))
	for _, row := range rows {
		out = append(out, AgentGrantView{
			UUID:           row.UUID,
			AgentUUID:      row.AgentUUID,
			CapabilityUUID: row.CapabilityUUID,
			PluginUUID:     row.PluginUUID,
			CapabilityID:   row.CapabilityID,
			PluginID:       row.PluginID,
			PermissionCode: row.PermissionCode,
			RiskLevel:      row.RiskLevel,
			Status:         row.Status,
			Source:         row.Source,
		})
	}
	return out
}

func grantableKey(capabilityUUID uuid.UUID, permissionCode string) string {
	return strings.ToLower(capabilityUUID.String() + "|" + strings.TrimSpace(permissionCode))
}

func stringListFromJSON(data []byte) []string {
	var out []string
	_ = json.Unmarshal(data, &out)
	return out
}

func stringListFromAny(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(anyString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func anyString(raw any) string {
	if raw == nil {
		return ""
	}
	return fmt.Sprint(raw)
}

func boolFromAny(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}
	return ""
}
