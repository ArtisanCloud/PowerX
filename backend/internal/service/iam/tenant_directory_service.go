package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AuthorizationReasonPermissionDenied = "IAM_PERMISSION_DENIED"
	authorizationResourceIAMMember      = "iam.member"
	authorizationActionRead             = "read"
)

const (
	DirectoryDisplayNameFound     = "found"
	DirectoryDisplayNameNotFound  = "not_found"
	DirectoryDisplayNameAmbiguous = "ambiguous"
)

// TenantDirectoryService is the read-only service boundary used by delegated
// plugin adapters. It exposes UUID-only DTOs and never accepts numeric IDs.
type TenantDirectoryService struct {
	db             *gorm.DB
	permissionRepo *iamrepo.PermissionRepository
}

func NewTenantDirectoryService(db *gorm.DB) *TenantDirectoryService {
	return &TenantDirectoryService{db: db, permissionRepo: iamrepo.NewPermissionRepository(db)}
}

type TenantDirectoryDepartment struct {
	DepartmentUUID       string  `json:"department_uuid"`
	TenantUUID           string  `json:"tenant_uuid"`
	Key                  string  `json:"key"`
	Name                 string  `json:"name"`
	ParentDepartmentUUID *string `json:"parent_department_uuid,omitempty"`
	LeaderMemberUUID     *string `json:"leader_member_uuid,omitempty"`
	Path                 string  `json:"path"`
	Depth                int     `json:"depth"`
	Sort                 int     `json:"sort"`
	Status               int16   `json:"status"`
}

type TenantDirectoryRole struct {
	RoleUUID    string `json:"role_uuid"`
	TenantUUID  string `json:"tenant_uuid"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Builtin     bool   `json:"builtin"`
}

type TenantDirectoryPermission struct {
	PermissionUUID string `json:"permission_uuid"`
	Module         string `json:"module"`
	Resource       string `json:"resource"`
	Action         string `json:"action"`
	Effect         string `json:"effect"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"`
	Source         string `json:"source,omitempty"`
}

type TenantDirectoryTenant struct {
	TenantUUID string `json:"tenant_uuid"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	Status     int16  `json:"status"`
	Type       string `json:"type"`
	Plan       string `json:"plan"`
}

// TenantDirectoryDisplayNameMember is the UUID-only result used by import
// adapters. Numeric storage IDs are deliberately never exposed.
type TenantDirectoryDisplayNameMember struct {
	MemberUUID  string `json:"member_uuid"`
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
}

type TenantDirectoryDisplayNameResolution struct {
	DisplayName string                             `json:"display_name"`
	Status      string                             `json:"status"`
	Members     []TenantDirectoryDisplayNameMember `json:"members"`
}

// FindMembersByDisplayNames performs exact display_name matching in the
// credential tenant. It preserves request order (including repeated names) so
// import callers can associate every row deterministically.
func (s *TenantDirectoryService) FindMembersByDisplayNames(ctx context.Context, tenantUUID string, displayNames []string) ([]TenantDirectoryDisplayNameResolution, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	tenantUUID, err := canonicalDirectoryTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	normalized := make([]string, 0, len(displayNames))
	unique := make([]string, 0, len(displayNames))
	seen := make(map[string]struct{}, len(displayNames))
	for _, raw := range displayNames {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, errors.New("IAM member display name is required")
		}
		normalized = append(normalized, name)
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			unique = append(unique, name)
		}
	}
	if len(normalized) == 0 {
		return nil, errors.New("IAM member display names are required")
	}

	var rows []struct {
		MemberUUID  uuid.UUID `gorm:"column:member_uuid"`
		UserUUID    string    `gorm:"column:user_uuid"`
		DisplayName string    `gorm:"column:display_name"`
	}
	if err := s.db.WithContext(ctx).Table((&modeliam.Member{}).GetTableName(true)).
		Select("uuid AS member_uuid, user_uuid, display_name").
		Where("tenant_uuid = ? AND username <> ? AND display_name IN ?", tenantUUID, ROOT_USERNAME, unique).
		Order("display_name ASC, uuid ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	byName := make(map[string][]TenantDirectoryDisplayNameMember, len(unique))
	for _, row := range rows {
		if row.MemberUUID == uuid.Nil || strings.TrimSpace(row.UserUUID) == "" || strings.TrimSpace(row.DisplayName) == "" {
			return nil, errors.New("IAM member UUID, user UUID, or display name is missing")
		}
		byName[row.DisplayName] = append(byName[row.DisplayName], TenantDirectoryDisplayNameMember{MemberUUID: row.MemberUUID.String(), UserUUID: strings.TrimSpace(row.UserUUID), DisplayName: row.DisplayName})
	}
	result := make([]TenantDirectoryDisplayNameResolution, 0, len(normalized))
	for _, name := range normalized {
		matches := byName[name]
		status := DirectoryDisplayNameNotFound
		if len(matches) == 1 {
			status = DirectoryDisplayNameFound
		} else if len(matches) > 1 {
			status = DirectoryDisplayNameAmbiguous
		}
		result = append(result, TenantDirectoryDisplayNameResolution{DisplayName: name, Status: status, Members: matches})
	}
	return result, nil
}

func (s *TenantDirectoryService) GetTenant(ctx context.Context, tenantUUID string) (TenantDirectoryTenant, error) {
	if err := s.requireDB(); err != nil {
		return TenantDirectoryTenant{}, err
	}
	tenantUUID, err := canonicalDirectoryTenantUUID(tenantUUID)
	if err != nil {
		return TenantDirectoryTenant{}, err
	}
	var tenant tenantmodel.Tenant
	if err := s.db.WithContext(ctx).Where("uuid = ?", tenantUUID).First(&tenant).Error; err != nil {
		return TenantDirectoryTenant{}, err
	}
	if tenant.UUID == uuid.Nil {
		return TenantDirectoryTenant{}, errors.New("IAM tenant UUID is missing")
	}
	return TenantDirectoryTenant{TenantUUID: tenant.UUID.String(), Key: tenant.Key, Name: tenant.Name, Status: tenant.Status, Type: tenant.Type, Plan: tenant.Plan}, nil
}

type AuthorizationCheckInput struct {
	MemberUUID string
	UserUUID   string
	Resource   string
	Action     string
	TraceID    string
}

type AuthorizationCheckResult struct {
	Allowed    bool   `json:"allowed"`
	ReasonCode string `json:"reason_code"`
	TenantUUID string `json:"tenant_uuid"`
	MemberUUID string `json:"member_uuid"`
	UserUUID   string `json:"user_uuid"`
	Resource   string `json:"resource"`
	Action     string `json:"action"`
	TraceID    string `json:"trace_id,omitempty"`
}

func (s *TenantDirectoryService) ListDepartments(ctx context.Context, tenantUUID string) ([]TenantDirectoryDepartment, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	tenantUUID, err := canonicalDirectoryTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	type row struct {
		DepartmentUUID       uuid.UUID  `gorm:"column:department_uuid"`
		TenantUUID           string     `gorm:"column:tenant_uuid"`
		Key                  string     `gorm:"column:key"`
		Name                 string     `gorm:"column:name"`
		ParentDepartmentUUID *uuid.UUID `gorm:"column:parent_department_uuid"`
		LeaderMemberUUID     *uuid.UUID `gorm:"column:leader_member_uuid"`
		Path                 string     `gorm:"column:path"`
		Depth                int        `gorm:"column:depth"`
		Sort                 int        `gorm:"column:sort"`
		Status               int16      `gorm:"column:status"`
	}
	var rows []row
	if err := s.db.WithContext(ctx).Table((&modeliam.Department{}).GetTableName(true)).
		Select("department_uuid, tenant_uuid, key, name, parent_department_uuid, leader_member_uuid, path, depth, sort, status").
		Where("tenant_uuid = ?", tenantUUID).Order("path ASC, sort ASC, department_uuid ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]TenantDirectoryDepartment, 0, len(rows))
	for _, item := range rows {
		if item.DepartmentUUID == uuid.Nil {
			return nil, fmt.Errorf("IAM department UUID is missing")
		}
		out := TenantDirectoryDepartment{DepartmentUUID: item.DepartmentUUID.String(), TenantUUID: item.TenantUUID, Key: item.Key, Name: item.Name, Path: item.Path, Depth: item.Depth, Sort: item.Sort, Status: item.Status}
		if item.ParentDepartmentUUID != nil {
			if *item.ParentDepartmentUUID == uuid.Nil {
				return nil, fmt.Errorf("IAM department parent UUID is missing")
			}
			value := item.ParentDepartmentUUID.String()
			out.ParentDepartmentUUID = &value
		}
		if item.LeaderMemberUUID != nil {
			if *item.LeaderMemberUUID == uuid.Nil {
				return nil, fmt.Errorf("IAM department leader UUID is missing")
			}
			value := item.LeaderMemberUUID.String()
			out.LeaderMemberUUID = &value
		}
		result = append(result, out)
	}
	return result, nil
}

func (s *TenantDirectoryService) ListRoles(ctx context.Context, tenantUUID string) ([]TenantDirectoryRole, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	tenantUUID, err := canonicalDirectoryTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	var roles []modeliam.Role
	if err := s.db.WithContext(ctx).Where("scope = ? AND tenant_uuid = ?", "tenant", tenantUUID).Order("builtin DESC, code ASC, uuid ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	result := make([]TenantDirectoryRole, 0, len(roles))
	for _, role := range roles {
		if role.UUID == uuid.Nil {
			return nil, fmt.Errorf("IAM role UUID is missing")
		}
		result = append(result, TenantDirectoryRole{RoleUUID: role.UUID.String(), TenantUUID: role.TenantUUID, Code: string(role.Code), Name: role.Name, Description: role.Description, Builtin: role.Builtin})
	}
	return result, nil
}

func (s *TenantDirectoryService) ListPermissions(ctx context.Context) ([]TenantDirectoryPermission, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	type row struct {
		PermissionUUID uuid.UUID `gorm:"column:permission_uuid"`
		Module         string    `gorm:"column:module"`
		Resource       string    `gorm:"column:resource"`
		Action         string    `gorm:"column:action"`
		Effect         string    `gorm:"column:effect"`
		Description    string    `gorm:"column:description"`
		Status         string    `gorm:"column:status"`
		Source         string    `gorm:"column:source"`
	}
	var rows []row
	if err := s.db.WithContext(ctx).Table((&modeliam.Permission{}).GetTableName(true)).
		Select("permission_uuid, module, resource, action, effect, description, status, source").
		Where("status = ?", modeliam.PermissionStatusActive).Order("module ASC, resource ASC, action ASC, permission_uuid ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]TenantDirectoryPermission, 0, len(rows))
	for _, item := range rows {
		if item.PermissionUUID == uuid.Nil {
			return nil, fmt.Errorf("IAM permission UUID is missing")
		}
		result = append(result, TenantDirectoryPermission{PermissionUUID: item.PermissionUUID.String(), Module: item.Module, Resource: item.Resource, Action: item.Action, Effect: item.Effect, Description: item.Description, Status: item.Status, Source: item.Source})
	}
	return result, nil
}

func (s *TenantDirectoryService) MemberDepartmentUUIDs(ctx context.Context, tenantUUID string, memberID uint64) ([]string, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	tenantUUID, err := canonicalDirectoryTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	var values []uuid.UUID
	if err := s.db.WithContext(ctx).Table(coremodel.TableIAMMemberDepartment).Where("tenant_uuid = ? AND member_id = ?", tenantUUID, memberID).Order("department_uuid ASC").Pluck("department_uuid", &values).Error; err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			return nil, fmt.Errorf("IAM member department UUID is missing")
		}
		result = append(result, value.String())
	}
	return result, nil
}

func (s *TenantDirectoryService) CheckAuthorization(ctx context.Context, tenantUUID string, input AuthorizationCheckInput) (AuthorizationCheckResult, error) {
	if err := s.requireDB(); err != nil {
		return AuthorizationCheckResult{}, err
	}
	tenantUUID, err := canonicalDirectoryTenantUUID(tenantUUID)
	if err != nil {
		return AuthorizationCheckResult{}, err
	}
	memberUUID, err := parseDirectoryUUID(input.MemberUUID)
	if err != nil {
		return AuthorizationCheckResult{}, err
	}
	userUUID, err := parseDirectoryUUID(input.UserUUID)
	if err != nil {
		return AuthorizationCheckResult{}, err
	}
	if strings.TrimSpace(input.Resource) != authorizationResourceIAMMember || strings.TrimSpace(input.Action) != authorizationActionRead {
		return AuthorizationCheckResult{}, errors.New("unsupported IAM authorization resource/action")
	}
	var member modeliam.Member
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, memberUUID).First(&member).Error; err != nil {
		return AuthorizationCheckResult{}, err
	}
	if strings.TrimSpace(member.UserUUID) != userUUID.String() {
		return AuthorizationCheckResult{}, DirectorySubjectMismatchError(errors.New("member and user do not match"))
	}
	allowed, err := s.permissionRepo.MemberHasPermissionViaUUIDBindingWithModule(ctx, tenantUUID, member.ID, member.UUID.String(), "corex.iam", "members", authorizationActionRead)
	if err != nil {
		return AuthorizationCheckResult{}, err
	}
	reason := ""
	if !allowed {
		reason = AuthorizationReasonPermissionDenied
	}
	return AuthorizationCheckResult{Allowed: allowed, ReasonCode: reason, TenantUUID: tenantUUID, MemberUUID: memberUUID.String(), UserUUID: userUUID.String(), Resource: authorizationResourceIAMMember, Action: authorizationActionRead, TraceID: strings.TrimSpace(input.TraceID)}, nil
}

func (s *TenantDirectoryService) requireDB() error {
	if s == nil || s.db == nil {
		return errors.New("IAM directory dependency unavailable")
	}
	return nil
}

func canonicalDirectoryTenantUUID(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}
func parseDirectoryUUID(value string) (uuid.UUID, error) { return uuid.Parse(strings.TrimSpace(value)) }
