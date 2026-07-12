package agent_authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
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
	db       *gorm.DB
	agents   *agentrepo.AgentRepository
	grants   *agentrepo.AgentCapabilityGrantRepository
	access   *agentrepo.AgentAccessGrantRepository
	rbac     *iamsvc.RBACService
	cache    cache.ICache
	cacheTTL time.Duration
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithCache(db, cache.GetCache())
}

func NewServiceWithCache(db *gorm.DB, store cache.ICache) *Service {
	return &Service{
		db:       db,
		agents:   agentrepo.NewAgentRepository(db),
		grants:   agentrepo.NewAgentCapabilityGrantRepository(db),
		access:   agentrepo.NewAgentAccessGrantRepository(db),
		rbac:     iamsvc.NewRBACService(db),
		cache:    store,
		cacheTTL: 30 * time.Second,
	}
}

type GrantableCapability struct {
	CapabilityUUID  uuid.UUID         `json:"capability_uuid"`
	CapabilityID    string            `json:"capability_id"`
	PluginID        string            `json:"plugin_id"`
	PluginUUID      *uuid.UUID        `json:"plugin_uuid,omitempty"`
	Module          string            `json:"module,omitempty"`
	DisplayName     string            `json:"display_name"`
	TitleI18n       map[string]string `json:"title_i18n,omitempty"`
	Description     string            `json:"description,omitempty"`
	DescriptionI18n map[string]string `json:"description_i18n,omitempty"`
	PermissionCode  string            `json:"permission_code"`
	RiskLevel       string            `json:"risk_level"`
	AgentUsable     bool              `json:"agent_usable"`
	TenantEnabled   bool              `json:"tenant_enabled"`
	Status          string            `json:"status"`
	Protocols       datatypes.JSON    `json:"-"`
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

type AgentAccessGrantInput struct {
	SubjectType string `json:"subject_type"`
	SubjectUUID string `json:"subject_uuid"`
	Enabled     bool   `json:"enabled"`
}

type AgentAccessGrantView struct {
	UUID        uuid.UUID `json:"uuid"`
	AgentUUID   uuid.UUID `json:"agent_uuid"`
	SubjectType string    `json:"subject_type"`
	SubjectUUID string    `json:"subject_uuid"`
	Status      string    `json:"status"`
	Source      string    `json:"source"`
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
	TenantUUID         string                    `json:"tenant_uuid"`
	UserUUID           string                    `json:"user_uuid"`
	MemberUUID         string                    `json:"member_uuid"`
	AgentUUID          uuid.UUID                 `json:"agent_uuid"`
	AgentAccessAllowed bool                      `json:"agent_access_allowed"`
	Items              []EffectivePermissionItem `json:"items"`
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
		module := moduleFromRecord(rec)
		titleI18n := annotationLocaleTextMap(rec, "title_i18n")
		descriptionI18n := annotationLocaleTextMap(rec, "description_i18n")
		for _, code := range permissionCodes {
			out = append(out, GrantableCapability{
				CapabilityUUID:  rec.UUID,
				CapabilityID:    rec.CapabilityID,
				PluginID:        rec.PluginID,
				Module:          module,
				DisplayName:     rec.Title,
				TitleI18n:       titleI18n,
				Description:     rec.Description,
				DescriptionI18n: descriptionI18n,
				PermissionCode:  code,
				RiskLevel:       risk,
				AgentUsable:     agentUsable,
				TenantEnabled:   tenantEnabled,
				Status:          rec.Status,
				Protocols:       rec.Protocols,
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

func (s *Service) PatchAgentGrants(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, actorUserUUID string, inputs []AgentGrantInput) ([]AgentGrantView, error) {
	agent, err := s.agentByUUID(ctx, env, tenantUUID, agentUUID)
	if err != nil {
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
		if baselineGrantAllowed(agent, item.PluginID) {
			return nil, fmt.Errorf("baseline capability grants cannot be modified: plugin_id=%s capability_id=%s", item.PluginID, item.CapabilityID)
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
	if err := s.grants.UpsertByAgent(ctx, env, tenantUUID, agentUUID, rows); err != nil {
		return nil, err
	}
	if err := s.invalidateAgentEffectivePermissionsCache(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	return s.ListAgentGrants(ctx, env, tenantUUID, agentUUID)
}

func (s *Service) ListAgentAccessGrants(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, subjectType string) ([]AgentAccessGrantView, error) {
	if err := s.requireAgent(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	subjectType = strings.TrimSpace(subjectType)
	if subjectType != "" && subjectType != agentmodel.AgentAccessGrantSubjectMember && subjectType != agentmodel.AgentAccessGrantSubjectRole {
		return nil, fmt.Errorf("invalid subject_type")
	}
	rows, err := s.access.ListByAgent(ctx, env, tenantUUID, agentUUID, subjectType)
	if err != nil {
		return nil, err
	}
	return accessGrantViews(rows), nil
}

func (s *Service) PatchAgentAccessGrants(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, actorUserUUID string, inputs []AgentAccessGrantInput) ([]AgentAccessGrantView, error) {
	if err := s.requireAgent(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	rows := make([]agentmodel.AgentAccessGrant, 0, len(inputs))
	seen := map[string]struct{}{}
	for _, in := range inputs {
		subjectType := strings.ToLower(strings.TrimSpace(in.SubjectType))
		subjectUUID := strings.TrimSpace(in.SubjectUUID)
		if subjectType != agentmodel.AgentAccessGrantSubjectMember && subjectType != agentmodel.AgentAccessGrantSubjectRole {
			return nil, fmt.Errorf("invalid subject_type")
		}
		if _, err := uuid.Parse(subjectUUID); err != nil {
			return nil, fmt.Errorf("subject_uuid must be valid")
		}
		if err := s.requireSubject(ctx, tenantUUID, subjectType, subjectUUID); err != nil {
			return nil, err
		}
		key := subjectType + ":" + strings.ToLower(subjectUUID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		status := agentmodel.AgentAccessGrantStatusDisabled
		if in.Enabled {
			status = agentmodel.AgentAccessGrantStatusEnabled
		}
		rows = append(rows, agentmodel.AgentAccessGrant{
			AgentUUID:         agentUUID,
			SubjectType:       subjectType,
			SubjectUUID:       subjectUUID,
			Status:            status,
			Source:            agentmodel.AgentAccessGrantSourceManual,
			CreatedByUserUUID: strings.TrimSpace(actorUserUUID),
			UpdatedByUserUUID: strings.TrimSpace(actorUserUUID),
		})
	}
	if err := s.access.UpsertByAgent(ctx, env, tenantUUID, agentUUID, rows); err != nil {
		return nil, err
	}
	if err := s.invalidateAgentEffectivePermissionsCache(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	return s.ListAgentAccessGrants(ctx, env, tenantUUID, agentUUID, "")
}

func (s *Service) ReplaceAgentGrants(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, actorUserUUID string, inputs []AgentGrantInput) ([]AgentGrantView, error) {
	agent, err := s.agentByUUID(ctx, env, tenantUUID, agentUUID)
	if err != nil {
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
		if baselineGrantAllowed(agent, item.PluginID) {
			return nil, fmt.Errorf("baseline capability grants cannot be modified: plugin_id=%s capability_id=%s", item.PluginID, item.CapabilityID)
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
	if err := s.invalidateAgentEffectivePermissionsCache(ctx, env, tenantUUID, agentUUID); err != nil {
		return nil, err
	}
	return s.ListAgentGrants(ctx, env, tenantUUID, agentUUID)
}

func (s *Service) ResolveEffectivePermissions(ctx context.Context, env, tenantUUID, userUUID, memberUUID string, memberID uint64, isRoot bool, agentUUID uuid.UUID) (EffectivePermissionsResult, error) {
	if cached, ok, err := s.getCachedEffectivePermissions(ctx, env, tenantUUID, userUUID, memberUUID, memberID, isRoot, agentUUID); err != nil {
		return EffectivePermissionsResult{}, err
	} else if ok {
		return cached, nil
	}
	agent, err := s.agentByUUID(ctx, env, tenantUUID, agentUUID)
	if err != nil {
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
	accessAllowed, err := s.agentAccessAllowed(ctx, env, tenantUUID, agentUUID, memberUUID, memberID, isRoot)
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
		userAllowed, parseOK, err := s.userAllowedForCapability(ctx, tenantUUID, memberID, isRoot, cap)
		if err != nil {
			return EffectivePermissionsResult{}, err
		}
		agAllowed := baselineGrantAllowed(agent, cap.PluginID) || agentAllowed[grantableKey(cap.CapabilityUUID, cap.PermissionCode)]
		policyAllowed := cap.AgentUsable
		effective := accessAllowed && userAllowed && agAllowed && cap.TenantEnabled && policyAllowed && parseOK
		deny := ""
		switch {
		case !accessAllowed:
			deny = "agent_access_grant_missing"
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
	result := EffectivePermissionsResult{
		TenantUUID:         tenantUUID,
		UserUUID:           userUUID,
		MemberUUID:         memberUUID,
		AgentUUID:          agentUUID,
		AgentAccessAllowed: accessAllowed,
		Items:              items,
	}
	if err := s.setCachedEffectivePermissions(ctx, env, tenantUUID, userUUID, memberUUID, memberID, isRoot, agentUUID, result); err != nil {
		return EffectivePermissionsResult{}, err
	}
	return result, nil
}

func (s *Service) AuthorizeCapability(ctx context.Context, in AuthorizeInput) (result AuthorizeResult, err error) {
	tenantUUID := strings.TrimSpace(firstNonEmpty(in.TenantUUID, reqctx.GetTenantUUID(ctx)))
	env := firstNonEmpty(strings.TrimSpace(in.Env), reqctx.GetEnv(ctx), "dev")
	agentUUID := in.AgentUUID
	capabilityID := strings.TrimSpace(in.CapabilityID)
	defer func() {
		memberID := in.MemberID
		if memberID == 0 {
			memberID = reqctx.GetMemberID(ctx)
		}
		fields := map[string]interface{}{
			"module":          "agent_authz",
			"event":           "agent.capability_authorize",
			"env":             env,
			"tenant_uuid":     tenantUUID,
			"agent_uuid":      agentUUID.String(),
			"agent_id":        in.AgentID,
			"user_uuid":       strings.TrimSpace(firstNonEmpty(in.UserUUID, reqctx.GetUserUUID(ctx))),
			"member_id":       memberID,
			"capability_id":   capabilityID,
			"permission_code": result.PermissionCode,
			"allowed":         result.Allowed,
			"deny_reason":     result.DenyReason,
		}
		logCtx := pxlog.WithLogFields(ctx, fields)
		if err != nil || !result.Allowed {
			pxlog.WarnF(logCtx, "agent capability authorization denied err=%v", err)
			return
		}
		pxlog.Info(logCtx, "agent capability authorization allowed")
	}()
	if tenantUUID == "" {
		return AuthorizeResult{Allowed: false, DenyReason: "tenant_missing"}, nil
	}
	if agentUUID == uuid.Nil {
		agent, err := s.agentByNumericID(ctx, in.AgentID, tenantUUID)
		if err != nil {
			return AuthorizeResult{Allowed: false, DenyReason: "agent_missing"}, err
		}
		agentUUID = agent.UUID
	}
	agent, err := s.agentByUUID(ctx, env, tenantUUID, agentUUID)
	if err != nil {
		return AuthorizeResult{Allowed: false, DenyReason: "agent_missing"}, err
	}
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
	module, resource, action, ok := ParsePermissionCodeForPlugin(permissionCode, rec.PluginID)
	if !ok {
		return AuthorizeResult{Allowed: false, DenyReason: "permission_code_invalid", PermissionCode: permissionCode}, nil
	}
	if !s.tenantCapabilityEnabled(ctx, tenantUUID, capabilityID) {
		return AuthorizeResult{Allowed: false, DenyReason: "tenant_capability_disabled", PermissionCode: permissionCode}, nil
	}
	if !baselineGrantAllowed(agent, rec.PluginID) {
		hasGrant, err := s.grants.HasEnabledGrant(ctx, env, tenantUUID, agentUUID, capabilityID, permissionCode)
		if err != nil {
			return AuthorizeResult{Allowed: false, DenyReason: "agent_grant_check_failed", PermissionCode: permissionCode}, err
		}
		if !hasGrant {
			return AuthorizeResult{Allowed: false, DenyReason: "agent_grant_missing", PermissionCode: permissionCode}, nil
		}
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

func (s *Service) requireSubject(ctx context.Context, tenantUUID, subjectType, subjectUUID string) error {
	switch subjectType {
	case agentmodel.AgentAccessGrantSubjectMember:
		var count int64
		err := s.db.WithContext(ctx).
			Model(&iammodel.Member{}).
			Where("tenant_uuid = ? AND uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(subjectUUID)).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("member not found")
		}
		return nil
	case agentmodel.AgentAccessGrantSubjectRole:
		var count int64
		err := s.db.WithContext(ctx).
			Model(&iammodel.Role{}).
			Where("tenant_uuid = ? AND uuid = ?", strings.TrimSpace(tenantUUID), strings.TrimSpace(subjectUUID)).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("role not found")
		}
		return nil
	default:
		return fmt.Errorf("invalid subject_type")
	}
}

func (s *Service) agentAccessAllowed(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, memberUUID string, memberID uint64, isRoot bool) (bool, error) {
	if isRoot {
		return true, nil
	}
	memberUUID = strings.TrimSpace(memberUUID)
	if memberUUID == "" || memberID == 0 {
		return false, nil
	}
	roleUUIDs, err := s.roleUUIDsForMember(ctx, tenantUUID, memberID)
	if err != nil {
		return false, err
	}
	return s.access.HasEnabledForSubjects(ctx, env, tenantUUID, agentUUID, map[string][]string{
		agentmodel.AgentAccessGrantSubjectMember: []string{memberUUID},
		agentmodel.AgentAccessGrantSubjectRole:   roleUUIDs,
	})
}

func (s *Service) roleUUIDsForMember(ctx context.Context, tenantUUID string, memberID uint64) ([]string, error) {
	var rows []struct {
		UUID uuid.UUID `gorm:"column:uuid"`
	}
	err := s.db.WithContext(ctx).
		Table((&iammodel.Role{}).GetTableName(true)+" AS r").
		Select("r.uuid").
		Joins("JOIN "+(&iammodel.RoleBinding{}).GetTableName(true)+" AS rb ON rb.role_id = r.id").
		Where("rb.tenant_uuid = ? AND rb.subject_type = ? AND rb.subject_id = ?", strings.TrimSpace(tenantUUID), iammodel.SubMember, memberID).
		Where("r.uuid IS NOT NULL").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.UUID != uuid.Nil {
			out = append(out, row.UUID.String())
		}
	}
	return out, nil
}

func (s *Service) userAllowedForCapability(ctx context.Context, tenantUUID string, memberID uint64, isRoot bool, cap GrantableCapability) (allowed bool, parseOK bool, err error) {
	requirements := iamRequirementsForGrantableCapability(cap)
	if len(requirements) == 0 {
		return false, false, nil
	}
	for _, req := range requirements {
		ok, enforceErr := s.rbac.Enforce(ctx, iamsvc.ActorContext{IsRoot: isRoot, TenantUUID: tenantUUID}, tenantUUID, memberID, req.Module, req.Resource, req.Action)
		if enforceErr != nil {
			return false, true, enforceErr
		}
		if !ok {
			return false, true, nil
		}
	}
	return true, true, nil
}

type iamRequirement struct {
	Module   string
	Resource string
	Action   string
}

func iamRequirementsForGrantableCapability(cap GrantableCapability) []iamRequirement {
	if isCoreCapabilityPlugin(cap.PluginID) {
		requirements := restIAMRequirementsFromCapability(cap)
		if len(requirements) > 0 {
			return requirements
		}
	}
	module, resource, action, ok := ParsePermissionCodeForPlugin(cap.PermissionCode, cap.PluginID)
	if !ok {
		return nil
	}
	return []iamRequirement{{Module: module, Resource: resource, Action: action}}
}

func restIAMRequirementsFromCapability(cap GrantableCapability) []iamRequirement {
	var protocols []capmodels.ProtocolBinding
	if err := json.Unmarshal(cap.Protocols, &protocols); err != nil {
		return nil
	}
	out := make([]iamRequirement, 0, len(protocols))
	seen := map[string]struct{}{}
	for _, protocol := range protocols {
		if !strings.EqualFold(strings.TrimSpace(protocol.Channel), "rest") {
			continue
		}
		module, resource, action, ok := apikeypermissions.RESTPermissionTriple(cap.Module, protocol.Method, protocol.Endpoint)
		if !ok {
			continue
		}
		key := module + "|" + resource + "|" + action
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, iamRequirement{Module: module, Resource: resource, Action: action})
	}
	return out
}

func (s *Service) agentByUUID(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID) (*agentmodel.Agent, error) {
	if strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || agentUUID == uuid.Nil {
		return nil, fmt.Errorf("env, tenant_uuid and agent_uuid are required")
	}
	tenantRef := strings.TrimSpace(tenantUUID)
	return s.agents.FindByScopeUUID(ctx, strings.TrimSpace(env), &tenantRef, agentUUID)
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
		if !strings.Contains(code, ":") && isCoreCapabilityPlugin(rec.PluginID) {
			code = corePermissionCodeFromScope(code)
		}
		if _, _, _, ok := ParsePermissionCodeForPlugin(code, rec.PluginID); !ok {
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

func corePermissionCodeFromScope(scope string) string {
	scope = strings.Trim(strings.ToLower(strings.TrimSpace(scope)), ".")
	if scope == "" {
		return ""
	}
	scope = strings.ReplaceAll(scope, ":", ".")
	return "corex." + scope + ":use"
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

func moduleFromRecord(rec capmodels.CapabilityRecord) string {
	var annotations map[string]any
	if err := json.Unmarshal(rec.Annotations, &annotations); err == nil {
		if v := strings.TrimSpace(anyString(annotations["module"])); v != "" {
			return v
		}
	}
	module, _, _, ok := ParsePermissionCodeForPlugin(firstPermissionCodeFromRecord(rec), rec.PluginID)
	if ok {
		return module
	}
	return strings.TrimSpace(rec.PluginID)
}

func firstPermissionCodeFromRecord(rec capmodels.CapabilityRecord) string {
	codes := permissionCodesFromRecord(rec)
	if len(codes) == 0 {
		return ""
	}
	return codes[0]
}

func baselineGrantAllowed(agent *agentmodel.Agent, pluginID string) bool {
	pluginID = strings.TrimSpace(pluginID)
	if isCoreCapabilityPlugin(pluginID) {
		return true
	}
	if agent == nil || agent.OwnerPluginID == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*agent.OwnerPluginID), pluginID)
}

func isCoreCapabilityPlugin(pluginID string) bool {
	normalized := strings.ToLower(strings.TrimSpace(pluginID))
	if normalized == "" {
		return true
	}
	return normalized == "core" ||
		normalized == "corex" ||
		normalized == "corex.platform" ||
		normalized == "powerx" ||
		strings.HasPrefix(normalized, "corex.") ||
		strings.HasPrefix(normalized, "com.powerx.core") ||
		strings.HasPrefix(normalized, "com.corex.")
}

func annotationLocaleTextMap(rec capmodels.CapabilityRecord, key string) map[string]string {
	var annotations map[string]any
	if err := json.Unmarshal(rec.Annotations, &annotations); err != nil {
		return nil
	}
	raw, ok := annotations[key].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for locale, value := range raw {
		locale = strings.TrimSpace(locale)
		text := strings.TrimSpace(anyString(value))
		if locale == "" || text == "" {
			continue
		}
		out[locale] = text
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func ParsePermissionCodeForPlugin(code string, pluginID string) (module, resource, action string, ok bool) {
	code = strings.TrimSpace(code)
	left, right, found := strings.Cut(code, ":")
	if !found {
		return "", "", "", false
	}
	action = strings.TrimSpace(right)
	left = strings.TrimSpace(left)
	pluginID = strings.TrimSpace(pluginID)
	if pluginID != "" {
		prefix := pluginID + "."
		if strings.HasPrefix(left, prefix) {
			resource = strings.TrimSpace(strings.TrimPrefix(left, prefix))
			if resource == "" || action == "" {
				return "", "", "", false
			}
			return pluginID, resource, action, true
		}
	}
	return ParsePermissionCode(code)
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

func accessGrantViews(rows []agentmodel.AgentAccessGrant) []AgentAccessGrantView {
	out := make([]AgentAccessGrantView, 0, len(rows))
	for _, row := range rows {
		out = append(out, AgentAccessGrantView{
			UUID:        row.UUID,
			AgentUUID:   row.AgentUUID,
			SubjectType: row.SubjectType,
			SubjectUUID: row.SubjectUUID,
			Status:      row.Status,
			Source:      row.Source,
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
