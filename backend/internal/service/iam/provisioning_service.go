package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PluginRoleCodePrefix = "plugin_"

var machineCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

type ProvisioningService struct {
	db    *gorm.DB
	audit auditsvc.Service
}

type ProvisionRoleInput struct {
	TenantUUID   string
	Code         string
	Name         string
	Description  string
	ActorSubject string
}

type ProvisionRoleResult struct {
	RoleUUID    string `json:"role_uuid"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Builtin     bool   `json:"builtin"`
	TenantUUID  string `json:"tenant_uuid"`
}

type ListProvisionRolesInput struct {
	TenantUUID     string
	Keyword        string
	Code           string
	IncludeBuiltin bool
	Page           int
	PageSize       int
}

type ProvisionRoleListItem struct {
	RoleUUID    string `json:"role_uuid"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Builtin     bool   `json:"builtin"`
	TenantUUID  string `json:"tenant_uuid"`
}

type ListProvisionRolesResult struct {
	Items    []ProvisionRoleListItem
	Total    int64
	Page     int
	PageSize int
}

type ProvisionMemberInput struct {
	TenantUUID       string
	Username         string
	Email            string
	DisplayName      string
	AvatarURL        string
	Status           string
	InitialPassword  string
	RoleCodes        []string
	Metadata         map[string]any
	SourceExternalID string
	ActorSubject     string
}

type ProvisionMemberResult struct {
	UserUUID   string   `json:"user_uuid"`
	MemberUUID string   `json:"member_uuid"`
	Username   string   `json:"username"`
	Email      string   `json:"email"`
	RoleCodes  []string `json:"role_codes"`
	TenantUUID string   `json:"tenant_uuid"`
}

func NewProvisioningService(db *gorm.DB, audit auditsvc.Service) *ProvisioningService {
	return &ProvisioningService{db: db, audit: audit}
}

func (s *ProvisioningService) ProvisionRole(ctx context.Context, in ProvisionRoleInput) (ProvisionRoleResult, error) {
	if s == nil || s.db == nil {
		return ProvisionRoleResult{}, errors.New("iam.provision.db_unavailable")
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return ProvisionRoleResult{}, err
	}
	code, err := normalizePluginRoleCode(in.Code)
	if err != nil {
		return ProvisionRoleResult{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ProvisionRoleResult{}, errors.New("iam.provision.role_name_required")
	}
	if len(name) > 128 {
		return ProvisionRoleResult{}, errors.New("iam.provision.role_name_too_long")
	}
	description := strings.TrimSpace(in.Description)
	if len(description) > 2000 {
		return ProvisionRoleResult{}, errors.New("iam.provision.role_description_too_long")
	}

	role := modeliam.Role{
		Scope:       string(coreiam.RoleScopeTenant),
		TenantUUID:  tenantUUID,
		Code:        coreiam.RoleCode(code),
		Name:        name,
		Description: description,
		Builtin:     false,
	}
	if err := s.db.WithContext(ctx).Create(&role).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ProvisionRoleResult{}, errors.New("iam.provision.role_code_conflict")
		}
		return ProvisionRoleResult{}, err
	}
	if role.UUID == uuid.Nil {
		return ProvisionRoleResult{}, errors.New("iam.provision.role_uuid_missing")
	}
	out := ProvisionRoleResult{
		RoleUUID:    role.UUID.String(),
		Code:        string(role.Code),
		Name:        role.Name,
		Description: role.Description,
		Builtin:     role.Builtin,
		TenantUUID:  role.TenantUUID,
	}
	s.emitAudit(ctx, role.TenantUUID, "iam.role.provision", "iam.role", role.UUID.String(), role.Name, map[string]any{
		"role_code":     string(role.Code),
		"actor_subject": strings.TrimSpace(in.ActorSubject),
	})
	return out, nil
}

func (s *ProvisioningService) ListProvisionRoles(ctx context.Context, in ListProvisionRolesInput) (ListProvisionRolesResult, error) {
	if s == nil || s.db == nil {
		return ListProvisionRolesResult{}, errors.New("iam.provision.db_unavailable")
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return ListProvisionRolesResult{}, err
	}
	page := in.Page
	if page <= 0 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	q := s.db.WithContext(ctx).
		Model(&modeliam.Role{}).
		Where("scope = ? AND tenant_uuid = ?", string(coreiam.RoleScopeTenant), tenantUUID)
	pluginRoleLikePattern := strings.ReplaceAll(PluginRoleCodePrefix, "_", `\_`) + "%"
	if in.IncludeBuiltin {
		q = q.Where("(code = ? OR code LIKE ? ESCAPE '\\')", string(coreiam.CodeRoleUser), pluginRoleLikePattern)
	} else {
		q = q.Where("code LIKE ? ESCAPE '\\'", pluginRoleLikePattern)
	}
	if code := strings.ToLower(strings.TrimSpace(in.Code)); code != "" {
		if code == string(coreiam.CodeRoleUser) {
			if !in.IncludeBuiltin {
				return ListProvisionRolesResult{Items: []ProvisionRoleListItem{}, Total: 0, Page: page, PageSize: pageSize}, nil
			}
		} else if _, err := normalizePluginRoleCode(code); err != nil {
			return ListProvisionRolesResult{}, errors.New("iam.provision.role_code_not_allowed")
		}
		q = q.Where("code = ?", code)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		q = q.Where("(LOWER(code) LIKE ? OR LOWER(name) LIKE ?)", like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ListProvisionRolesResult{}, err
	}
	var roles []modeliam.Role
	if err := q.Order("builtin DESC, code ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&roles).Error; err != nil {
		return ListProvisionRolesResult{}, err
	}
	items := make([]ProvisionRoleListItem, 0, len(roles))
	for _, role := range roles {
		if role.UUID == uuid.Nil {
			return ListProvisionRolesResult{}, errors.New("iam.provision.role_uuid_missing")
		}
		items = append(items, ProvisionRoleListItem{
			RoleUUID:    role.UUID.String(),
			Code:        string(role.Code),
			Name:        role.Name,
			Description: role.Description,
			Builtin:     role.Builtin,
			TenantUUID:  role.TenantUUID,
		})
	}
	return ListProvisionRolesResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ProvisioningService) ProvisionMember(ctx context.Context, in ProvisionMemberInput) (ProvisionMemberResult, error) {
	if s == nil || s.db == nil {
		return ProvisionMemberResult{}, errors.New("iam.provision.db_unavailable")
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return ProvisionMemberResult{}, err
	}
	username, email, displayName, status, err := normalizeMemberProvisionInput(in)
	if err != nil {
		return ProvisionMemberResult{}, err
	}
	roleCodes, err := normalizeProvisionRoleCodes(in.RoleCodes)
	if err != nil {
		return ProvisionMemberResult{}, err
	}
	meta, err := memberProvisionMeta(in.Metadata, in.SourceExternalID)
	if err != nil {
		return ProvisionMemberResult{}, err
	}

	var out ProvisionMemberResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, created, err := findOrCreateProvisionUser(ctx, tx, email, displayName, status, meta)
		if err != nil {
			return err
		}
		if user.IsRoot {
			return errors.New("iam.provision.root_user_forbidden")
		}
		if user.UUID == uuid.Nil {
			return errors.New("iam.provision.user_uuid_missing")
		}
		if err := ensureProvisionCredential(ctx, tx, user.ID, email, in.InitialPassword, created); err != nil {
			return err
		}
		member, err := ensureProvisionMember(ctx, tx, tenantUUID, user, username, displayName, in.AvatarURL, status, meta)
		if err != nil {
			return err
		}
		if err := bindProvisionRoles(ctx, tx, tenantUUID, member, roleCodes); err != nil {
			return err
		}
		out = ProvisionMemberResult{
			UserUUID:   user.UUID.String(),
			MemberUUID: member.UUID.String(),
			Username:   member.Username,
			Email:      user.Email,
			RoleCodes:  roleCodes,
			TenantUUID: tenantUUID,
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	s.emitAudit(ctx, out.TenantUUID, "iam.member.provision", "iam.member", out.MemberUUID, out.Username, map[string]any{
		"user_uuid":          out.UserUUID,
		"email":              out.Email,
		"role_codes":         out.RoleCodes,
		"actor_subject":      strings.TrimSpace(in.ActorSubject),
		"source_external_id": strings.TrimSpace(in.SourceExternalID),
	})
	return out, nil
}

func normalizeMemberProvisionInput(in ProvisionMemberInput) (username, email, displayName string, status int16, err error) {
	username = strings.ToLower(strings.TrimSpace(in.Username))
	if username == "" {
		return "", "", "", 0, errors.New("iam.provision.username_required")
	}
	if username == ROOT_USERNAME {
		return "", "", "", 0, errors.New("iam.provision.root_username_forbidden")
	}
	if !machineCodePattern.MatchString(username) {
		return "", "", "", 0, errors.New("iam.provision.username_invalid")
	}
	email = strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		return "", "", "", 0, errors.New("iam.provision.email_required")
	}
	if !strings.Contains(email, "@") || len(email) > 128 {
		return "", "", "", 0, errors.New("iam.provision.email_invalid")
	}
	displayName = strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return "", "", "", 0, errors.New("iam.provision.display_name_required")
	}
	if len(displayName) > 128 {
		return "", "", "", 0, errors.New("iam.provision.display_name_too_long")
	}
	switch strings.ToLower(strings.TrimSpace(in.Status)) {
	case "", "active", "enabled":
		status = modeliam.UserStatusActive
	case "inactive", "disabled":
		status = modeliam.UserStatusInactive
	default:
		return "", "", "", 0, errors.New("iam.provision.status_invalid")
	}
	if pw := strings.TrimSpace(in.InitialPassword); pw != "" && len(pw) < 8 {
		return "", "", "", 0, errors.New("iam.provision.initial_password_too_short")
	}
	return username, email, displayName, status, nil
}

func normalizePluginRoleCode(raw string) (string, error) {
	code := strings.ToLower(strings.TrimSpace(raw))
	if code == "" {
		return "", errors.New("iam.provision.role_code_required")
	}
	if !machineCodePattern.MatchString(code) {
		return "", errors.New("iam.provision.role_code_invalid")
	}
	if !strings.HasPrefix(code, PluginRoleCodePrefix) {
		return "", fmt.Errorf("iam.provision.role_code_prefix_required:%s", PluginRoleCodePrefix)
	}
	if code == PluginRoleCodePrefix {
		return "", errors.New("iam.provision.role_code_suffix_required")
	}
	switch coreiam.RoleCode(code) {
	case coreiam.CodeSystemAdmin, coreiam.CodeSystemMonitor, coreiam.CodeRoleOwner, coreiam.CodeRoleAdmin, coreiam.CodeRoleUser, coreiam.CodeRoleReadonly, coreiam.CodeRoleVendor:
		return "", errors.New("iam.provision.reserved_role_code")
	}
	return code, nil
}

func normalizeProvisionRoleCodes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{string(coreiam.CodeRoleUser)}, nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		code := strings.ToLower(strings.TrimSpace(item))
		if code == "" {
			continue
		}
		if code != string(coreiam.CodeRoleUser) {
			if _, err := normalizePluginRoleCode(code); err != nil {
				return nil, errors.New("iam.provision.role_code_not_allowed")
			}
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	if len(result) == 0 {
		return []string{string(coreiam.CodeRoleUser)}, nil
	}
	sort.Strings(result)
	return result, nil
}

func memberProvisionMeta(input map[string]any, sourceExternalID string) (datatypes.JSON, error) {
	meta := map[string]any{}
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		meta[key] = value
	}
	if source := strings.TrimSpace(sourceExternalID); source != "" {
		meta["source_external_id"] = source
	}
	if len(meta) == 0 {
		return datatypes.JSON([]byte("{}")), nil
	}
	return datatypes.JSONMap(meta).MarshalJSON()
}

func findOrCreateProvisionUser(ctx context.Context, tx *gorm.DB, email, displayName string, status int16, meta datatypes.JSON) (*modeliam.User, bool, error) {
	var user modeliam.User
	err := tx.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err == nil {
		return &user, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	user = modeliam.User{
		Email:       email,
		DisplayName: displayName,
		Status:      status,
		Meta:        meta,
	}
	if err := tx.WithContext(ctx).Create(&user).Error; err != nil {
		return nil, false, err
	}
	if user.UUID == uuid.Nil {
		return nil, false, errors.New("iam.provision.user_uuid_missing")
	}
	return &user, true, nil
}

func ensureProvisionCredential(ctx context.Context, tx *gorm.DB, userID uint64, identifier, initialPassword string, userCreated bool) error {
	password := strings.TrimSpace(initialPassword)
	if password == "" {
		if userCreated {
			return errors.New("iam.provision.initial_password_required")
		}
		return nil
	}
	var existing modeliam.Credential
	err := tx.WithContext(ctx).
		Where("provider = ? AND identifier = ?", "password", identifier).
		First(&existing).Error
	if err == nil {
		if existing.UserID != userID {
			return errors.New("iam.provision.credential_conflict")
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Create(&modeliam.Credential{
		UserID:     userID,
		Provider:   "password",
		Identifier: identifier,
		SecretHash: string(hash),
		IsPrimary:  true,
	}).Error
}

func ensureProvisionMember(ctx context.Context, tx *gorm.DB, tenantUUID string, user *modeliam.User, username, displayName, avatarURL string, status int16, meta datatypes.JSON) (*modeliam.Member, error) {
	var existing modeliam.Member
	err := tx.WithContext(ctx).
		Where("tenant_uuid = ? AND user_uuid = ?", tenantUUID, user.UUID.String()).
		First(&existing).Error
	if err == nil {
		if existing.UUID == uuid.Nil {
			return nil, errors.New("iam.provision.member_uuid_missing")
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	err = tx.WithContext(ctx).
		Where("tenant_uuid = ? AND username = ?", tenantUUID, username).
		First(&existing).Error
	if err == nil {
		return nil, errors.New("iam.provision.username_conflict")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	member := modeliam.Member{
		TenantUUID:  tenantUUID,
		UserUUID:    user.UUID.String(),
		UserID:      user.ID,
		Username:    username,
		DisplayName: displayName,
		AvatarURL:   strings.TrimSpace(avatarURL),
		Status:      status,
		Meta:        meta,
	}
	if err := tx.WithContext(ctx).Create(&member).Error; err != nil {
		return nil, err
	}
	if member.UUID == uuid.Nil {
		return nil, errors.New("iam.provision.member_uuid_missing")
	}
	return &member, nil
}

func bindProvisionRoles(ctx context.Context, tx *gorm.DB, tenantUUID string, member *modeliam.Member, codes []string) error {
	var roles []modeliam.Role
	if err := tx.WithContext(ctx).
		Where("scope = ? AND tenant_uuid = ? AND code IN ?", string(coreiam.RoleScopeTenant), tenantUUID, codes).
		Find(&roles).Error; err != nil {
		return err
	}
	byCode := make(map[string]modeliam.Role, len(roles))
	for _, role := range roles {
		if role.UUID == uuid.Nil {
			return errors.New("iam.provision.role_uuid_missing")
		}
		byCode[string(role.Code)] = role
	}
	for _, code := range codes {
		if _, ok := byCode[code]; !ok {
			return fmt.Errorf("iam.provision.role_not_found:%s", code)
		}
	}
	roleUUIDs := make([]string, 0, len(codes))
	for _, code := range codes {
		roleUUIDs = append(roleUUIDs, byCode[code].UUID.String())
	}
	var existing []modeliam.RoleBinding
	if err := tx.WithContext(ctx).
		Where("tenant_uuid = ? AND subject_uuid = ? AND role_uuid IN ?", tenantUUID, member.UUID.String(), roleUUIDs).
		Find(&existing).Error; err != nil {
		return err
	}
	exists := make(map[string]struct{}, len(existing))
	for _, binding := range existing {
		exists[binding.RoleUUID] = struct{}{}
	}
	rows := make([]modeliam.RoleBinding, 0, len(codes))
	for _, code := range codes {
		role := byCode[code]
		if _, ok := exists[role.UUID.String()]; ok {
			continue
		}
		rows = append(rows, modeliam.RoleBinding{
			TenantUUID:  tenantUUID,
			RoleUUID:    role.UUID.String(),
			RoleID:      role.ID,
			SubjectType: modeliam.SubMember,
			SubjectUUID: member.UUID.String(),
			SubjectID:   member.ID,
			DataScope:   modeliam.ScopeTenant,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (s *ProvisioningService) emitAudit(ctx context.Context, tenantUUID, operation, resourceType, resourceID, resourceName string, meta map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	payload, _ := json.Marshal(meta)
	actorSubject := ""
	if meta != nil {
		actorSubject = strings.TrimSpace(fmt.Sprint(meta["actor_subject"]))
	}
	if actorSubject == "<nil>" {
		actorSubject = ""
	}
	_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
		OccurredAt:    time.Now().UTC(),
		TenantUUID:    tenantUUID,
		CorrelationID: reqctx.GetTraceID(ctx),
		Source:        "iam.provisioning",
		Operation:     operation,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		ResourceName:  resourceName,
		Outcome:       "SUCCESS",
		Severity:      "INFO",
		ActorDisplay:  actorSubject,
		Meta:          datatypes.JSON(payload),
	})
}
