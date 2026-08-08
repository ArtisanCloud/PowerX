package iam

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MigrationStatusOK      = "ok"
	MigrationStatusMissing = "missing"
	MigrationStatusInvalid = "invalid"
)

type IAMMigrationReportService struct {
	*service.BaseService
	clock func() time.Time
}

type IAMMigrationUserSummary struct {
	ID          uint64 `json:"id"`
	UUID        string `json:"uuid"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Status      int16  `json:"status"`
	IsRoot      bool   `json:"is_root"`
}

type IAMMigrationReport struct {
	RootUsers              []IAMMigrationUserSummary `json:"root_users"`
	SystemTenantStatus     string                    `json:"system_tenant_status"`
	RootSystemMemberStatus string                    `json:"root_system_member_status"`
	TenantOwnerMissing     []string                  `json:"tenant_owner_missing"`
	TenantAdminMissing     []string                  `json:"tenant_admin_missing"`
	AutoFixCandidates      []string                  `json:"auto_fix_candidates"`
	ManualFixRequired      []string                  `json:"manual_fix_required"`
	DuplicateRoleBindings  []IAMRoleBindingDuplicate `json:"duplicate_role_bindings"`
}

type IAMRoleBindingDuplicate struct {
	TenantUUID     string `json:"tenant_uuid"`
	SubjectUUID    string `json:"subject_uuid"`
	RoleUUID       string `json:"role_uuid"`
	DataScope      string `json:"data_scope"`
	DuplicateCount int64  `json:"duplicate_count"`
	RowIDs         string `json:"row_ids"`
}

type IAMMigrationFixOwnerResult struct {
	FixedTenantUUIDs []string            `json:"fixed_tenant_uuids"`
	Report           *IAMMigrationReport `json:"report"`
}

type IAMMigrationFixDuplicateRoleBindingsResult struct {
	DryRun                   bool                      `json:"dry_run"`
	DuplicateGroups          []IAMRoleBindingDuplicate `json:"duplicate_groups"`
	DeletedIDs               []uint64                  `json:"deleted_ids"`
	KeptIDs                  []uint64                  `json:"kept_ids"`
	RemainingDuplicateGroups []IAMRoleBindingDuplicate `json:"remaining_duplicate_groups"`
}

func NewIAMMigrationReportService(db *gorm.DB) *IAMMigrationReportService {
	return &IAMMigrationReportService{
		BaseService: &service.BaseService{DB: db},
		clock:       time.Now,
	}
}

func (s *IAMMigrationReportService) Report(ctx context.Context) (*IAMMigrationReport, error) {
	if s == nil || s.DB == nil {
		return nil, dto.NewInternal("IAM migration report service 未初始化", nil)
	}
	report := &IAMMigrationReport{
		SystemTenantStatus:     MigrationStatusMissing,
		RootSystemMemberStatus: MigrationStatusMissing,
	}

	rootUsers, err := s.listRootUsers(ctx, s.DB)
	if err != nil {
		return nil, dto.NewInternal("查询 root 用户失败", err)
	}
	report.RootUsers = rootUsers

	systemTenant, err := s.findSystemTenant(ctx, s.DB)
	if err != nil {
		return nil, dto.NewInternal("查询 system tenant 失败", err)
	}
	if systemTenant != nil {
		if systemTenant.Status == modeltenant.TenantStatusActive && strings.EqualFold(systemTenant.Type, modeltenant.TenantTypeSystem) {
			report.SystemTenantStatus = MigrationStatusOK
		} else {
			report.SystemTenantStatus = MigrationStatusInvalid
		}
	}
	report.RootSystemMemberStatus = s.rootSystemMemberStatus(ctx, s.DB, systemTenant, rootUsers)
	duplicates, err := s.listDuplicateRoleBindings(ctx, s.DB)
	if err != nil {
		return nil, dto.NewInternal("查询重复角色绑定失败", err)
	}
	report.DuplicateRoleBindings = duplicates

	tenants, err := s.listBusinessTenants(ctx, s.DB)
	if err != nil {
		return nil, dto.NewInternal("查询业务租户失败", err)
	}
	for _, tenant := range tenants {
		tenantUUID := tenant.UUID.String()
		hasOwner, err := s.tenantHasActiveRole(ctx, s.DB, tenantUUID, coreiam.CodeRoleOwner)
		if err != nil {
			return nil, dto.NewInternal("查询租户 owner 状态失败", err)
		}
		hasAdmin, err := s.tenantHasActiveRole(ctx, s.DB, tenantUUID, coreiam.CodeRoleAdmin)
		if err != nil {
			return nil, dto.NewInternal("查询租户 admin 状态失败", err)
		}
		if !hasOwner {
			report.TenantOwnerMissing = append(report.TenantOwnerMissing, tenantUUID)
			if hasAdmin {
				report.AutoFixCandidates = append(report.AutoFixCandidates, tenantUUID)
			}
		}
		if !hasAdmin {
			report.TenantAdminMissing = append(report.TenantAdminMissing, tenantUUID)
			report.ManualFixRequired = append(report.ManualFixRequired, tenantUUID)
		}
	}
	sortReport(report)
	return report, nil
}

func (s *IAMMigrationReportService) FixMissingOwners(ctx context.Context) (*IAMMigrationFixOwnerResult, error) {
	return s.fixMissingOwners(ctx, true)
}

func (s *IAMMigrationReportService) FixMissingOwnersAsSystem(ctx context.Context) (*IAMMigrationFixOwnerResult, error) {
	return s.fixMissingOwners(ctx, false)
}

func (s *IAMMigrationReportService) FixDuplicateRoleBindingsAsSystem(ctx context.Context, confirm bool) (*IAMMigrationFixDuplicateRoleBindingsResult, error) {
	return s.fixDuplicateRoleBindings(ctx, false, confirm)
}

func (s *IAMMigrationReportService) FixDuplicateRoleBindings(ctx context.Context, confirm bool) (*IAMMigrationFixDuplicateRoleBindingsResult, error) {
	return s.fixDuplicateRoleBindings(ctx, true, confirm)
}

func (s *IAMMigrationReportService) fixDuplicateRoleBindings(ctx context.Context, requireRoot bool, confirm bool) (*IAMMigrationFixDuplicateRoleBindingsResult, error) {
	if s == nil || s.DB == nil {
		return nil, dto.NewInternal("IAM migration report service 未初始化", nil)
	}
	if requireRoot {
		if err := s.requireRootActor(ctx); err != nil {
			return nil, err
		}
	}

	result := &IAMMigrationFixDuplicateRoleBindingsResult{DryRun: !confirm}
	duplicates, err := s.listDuplicateRoleBindings(ctx, s.DB)
	if err != nil {
		return nil, dto.NewInternal("查询重复角色绑定失败", err)
	}
	result.DuplicateGroups = duplicates
	if !confirm {
		return result, nil
	}

	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deleteIDs := make([]uint64, 0)
		keepIDs := make([]uint64, 0, len(duplicates))
		for _, duplicate := range duplicates {
			ids, err := parseIAMRoleBindingRowIDs(duplicate.RowIDs)
			if err != nil {
				return err
			}
			if len(ids) <= 1 {
				continue
			}
			keepIDs = append(keepIDs, ids[0])
			deleteIDs = append(deleteIDs, ids[1:]...)
		}
		if len(deleteIDs) == 0 {
			result.KeptIDs = keepIDs
			return nil
		}
		if err := tx.Where("id IN ?", deleteIDs).Delete(&modeliam.RoleBinding{}).Error; err != nil {
			return dto.NewInternal("删除重复角色绑定失败", err)
		}
		result.DeletedIDs = deleteIDs
		result.KeptIDs = keepIDs
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result.DeletedIDs, func(i, j int) bool { return result.DeletedIDs[i] < result.DeletedIDs[j] })
	sort.Slice(result.KeptIDs, func(i, j int) bool { return result.KeptIDs[i] < result.KeptIDs[j] })
	remaining, err := s.listDuplicateRoleBindings(ctx, s.DB)
	if err != nil {
		return nil, dto.NewInternal("查询剩余重复角色绑定失败", err)
	}
	result.RemainingDuplicateGroups = remaining
	return result, nil
}

func (s *IAMMigrationReportService) fixMissingOwners(ctx context.Context, requireRoot bool) (*IAMMigrationFixOwnerResult, error) {
	if s == nil || s.DB == nil {
		return nil, dto.NewInternal("IAM migration report service 未初始化", nil)
	}
	if requireRoot {
		if err := s.requireRootActor(ctx); err != nil {
			return nil, err
		}
	}

	result := &IAMMigrationFixOwnerResult{}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		report, err := s.reportWithDB(ctx, tx)
		if err != nil {
			return err
		}
		for _, tenantUUID := range report.AutoFixCandidates {
			fixed, err := s.fixTenantOwner(ctx, tx, tenantUUID)
			if err != nil {
				return err
			}
			if fixed {
				result.FixedTenantUUIDs = append(result.FixedTenantUUIDs, tenantUUID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	report, err := s.Report(ctx)
	if err != nil {
		return nil, err
	}
	sort.Strings(result.FixedTenantUUIDs)
	result.Report = report
	return result, nil
}

func (s *IAMMigrationReportService) reportWithDB(ctx context.Context, db *gorm.DB) (*IAMMigrationReport, error) {
	clone := &IAMMigrationReportService{BaseService: &service.BaseService{DB: db}, clock: s.clock}
	return clone.Report(ctx)
}

func (s *IAMMigrationReportService) requireRootActor(ctx context.Context) error {
	userID := reqctx.GetUserID(ctx)
	if userID == 0 {
		return dto.NewUnauthorized("未登录", nil)
	}
	var row struct {
		IsRoot bool `gorm:"column:is_root"`
	}
	err := s.DB.WithContext(ctx).Model(&modeliam.User{}).Select("is_root").Where("id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.NewErrorWithCode(http.StatusForbidden, "IAM_MIGRATION_ROOT_REQUIRED", "仅 root 可执行 IAM 迁移修复", err)
	}
	if err != nil {
		return dto.NewInternal("校验 root 身份失败", err)
	}
	if !row.IsRoot {
		return dto.NewErrorWithCode(http.StatusForbidden, "IAM_MIGRATION_ROOT_REQUIRED", "仅 root 可执行 IAM 迁移修复", nil)
	}
	return nil
}

func (s *IAMMigrationReportService) listRootUsers(ctx context.Context, db *gorm.DB) ([]IAMMigrationUserSummary, error) {
	var users []modeliam.User
	if err := db.WithContext(ctx).Where("is_root = ?", true).Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]IAMMigrationUserSummary, 0, len(users))
	for _, user := range users {
		out = append(out, IAMMigrationUserSummary{
			ID:          user.ID,
			UUID:        user.UUID.String(),
			Email:       user.Email,
			Phone:       user.Phone,
			DisplayName: user.DisplayName,
			Status:      user.Status,
			IsRoot:      user.IsRoot,
		})
	}
	return out, nil
}

func (s *IAMMigrationReportService) findSystemTenant(ctx context.Context, db *gorm.DB) (*modeltenant.Tenant, error) {
	var tenant modeltenant.Tenant
	err := db.WithContext(ctx).Where("key = ?", modeltenant.SystemTenantKey).Take(&tenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

func (s *IAMMigrationReportService) rootSystemMemberStatus(ctx context.Context, db *gorm.DB, systemTenant *modeltenant.Tenant, rootUsers []IAMMigrationUserSummary) string {
	if systemTenant == nil || len(rootUsers) == 0 {
		return MigrationStatusMissing
	}
	rootIDs := make([]uint64, 0, len(rootUsers))
	for _, user := range rootUsers {
		rootIDs = append(rootIDs, user.ID)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&modeliam.Member{}).
		Where("tenant_uuid = ? AND user_id IN ? AND status = ?", systemTenant.UUID.String(), rootIDs, modeliam.UserStatusActive).
		Count(&count).Error; err != nil {
		return MigrationStatusInvalid
	}
	if count == 0 {
		return MigrationStatusMissing
	}
	return MigrationStatusOK
}

func (s *IAMMigrationReportService) listBusinessTenants(ctx context.Context, db *gorm.DB) ([]modeltenant.Tenant, error) {
	var tenants []modeltenant.Tenant
	err := db.WithContext(ctx).
		Where("key <> ?", modeltenant.SystemTenantKey).
		Where("type <> ?", modeltenant.TenantTypeSystem).
		Where("status = ?", modeltenant.TenantStatusActive).
		Order("id ASC").
		Find(&tenants).Error
	return tenants, err
}

func (s *IAMMigrationReportService) listDuplicateRoleBindings(ctx context.Context, db *gorm.DB) ([]IAMRoleBindingDuplicate, error) {
	table := (&modeliam.RoleBinding{}).GetTableName(true)
	trimFunc := "trim"
	rowIDsExpr := "group_concat(CAST(id AS TEXT), ',')"
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		trimFunc = "btrim"
		rowIDsExpr = "string_agg(id::text, ',' ORDER BY id)"
	}
	query := `
		SELECT tenant_uuid, subject_uuid, role_uuid, data_scope, COUNT(*) AS duplicate_count, ` + rowIDsExpr + ` AS row_ids
		FROM ` + table + `
		WHERE subject_type = ?
		  AND data_scope = ?
		  AND role_uuid IS NOT NULL
		  AND ` + trimFunc + `(role_uuid) <> ''
		  AND subject_uuid IS NOT NULL
		  AND ` + trimFunc + `(subject_uuid) <> ''
		GROUP BY tenant_uuid, subject_uuid, role_uuid, data_scope
		HAVING COUNT(*) > 1
		ORDER BY duplicate_count DESC, tenant_uuid, subject_uuid, role_uuid
	`
	var out []IAMRoleBindingDuplicate
	if err := db.WithContext(ctx).Raw(query, modeliam.SubMember, modeliam.ScopeTenant).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func parseIAMRoleBindingRowIDs(value string) ([]uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]uint64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, dto.NewInternal("重复角色绑定 row_ids 格式无效", err)
		}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (s *IAMMigrationReportService) tenantHasActiveRole(ctx context.Context, db *gorm.DB, tenantUUID string, code coreiam.RoleCode) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Table((&modeliam.RoleBinding{}).GetTableName(true)+" AS rb").
		Joins("JOIN "+(&modeliam.Role{}).GetTableName(true)+" AS r ON r.id = rb.role_id").
		Joins("JOIN "+(&modeliam.Member{}).GetTableName(true)+" AS m ON m.id = rb.subject_id AND m.tenant_uuid = rb.tenant_uuid").
		Where("rb.tenant_uuid = ? AND rb.subject_type = ? AND r.scope = ? AND r.tenant_uuid = ? AND r.code = ? AND m.status = ?",
			tenantUUID,
			modeliam.SubMember,
			string(coreiam.RoleScopeTenant),
			tenantUUID,
			code,
			modeliam.UserStatusActive,
		).
		Count(&count).Error
	return count > 0, err
}

func (s *IAMMigrationReportService) fixTenantOwner(ctx context.Context, db *gorm.DB, tenantUUID string) (bool, error) {
	ownerRole, err := s.findTenantRole(ctx, db, tenantUUID, coreiam.CodeRoleOwner)
	if err != nil {
		return false, err
	}
	adminMemberID, err := s.firstActiveAdminMemberID(ctx, db, tenantUUID)
	if err != nil {
		return false, err
	}
	if adminMemberID == 0 {
		return false, nil
	}
	if ownerRole.UUID == uuid.Nil {
		return false, dto.NewInternal("租户 owner 角色缺少 UUID", nil)
	}
	var adminMember modeliam.Member
	if err := db.WithContext(ctx).
		Where("tenant_uuid = ? AND id = ?", tenantUUID, adminMemberID).
		Take(&adminMember).Error; err != nil {
		return false, dto.NewInternal("查询租户管理员成员失败", err)
	}
	if adminMember.UUID == uuid.Nil {
		return false, dto.NewInternal("租户管理员成员缺少 UUID", nil)
	}
	binding := &modeliam.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleUUID:    ownerRole.UUID.String(),
		RoleID:      ownerRole.ID,
		SubjectType: modeliam.SubMember,
		SubjectUUID: adminMember.UUID.String(),
		SubjectID:   adminMemberID,
		DataScope:   modeliam.ScopeTenant,
	}
	res := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(binding)
	if res.Error != nil {
		return false, dto.NewInternal("补齐租户 owner 失败", res.Error)
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	if err := s.writeFixOwnerAudit(ctx, db, tenantUUID, adminMemberID, ownerRole.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *IAMMigrationReportService) findTenantRole(ctx context.Context, db *gorm.DB, tenantUUID string, code coreiam.RoleCode) (*modeliam.Role, error) {
	var role modeliam.Role
	err := db.WithContext(ctx).
		Where("scope = ? AND tenant_uuid = ? AND code = ?", string(coreiam.RoleScopeTenant), tenantUUID, code).
		Take(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.NewErrorWithCode(http.StatusConflict, "IAM_MIGRATION_ROLE_MISSING", "租户默认角色缺失", err)
	}
	if err != nil {
		return nil, dto.NewInternal("查询租户角色失败", err)
	}
	return &role, nil
}

func (s *IAMMigrationReportService) firstActiveAdminMemberID(ctx context.Context, db *gorm.DB, tenantUUID string) (uint64, error) {
	var row struct {
		MemberID uint64 `gorm:"column:member_id"`
	}
	err := db.WithContext(ctx).Table((&modeliam.RoleBinding{}).GetTableName(true)+" AS rb").
		Select("m.id AS member_id").
		Joins("JOIN "+(&modeliam.Role{}).GetTableName(true)+" AS r ON r.id = rb.role_id").
		Joins("JOIN "+(&modeliam.Member{}).GetTableName(true)+" AS m ON m.id = rb.subject_id AND m.tenant_uuid = rb.tenant_uuid").
		Where("rb.tenant_uuid = ? AND rb.subject_type = ? AND r.scope = ? AND r.tenant_uuid = ? AND r.code = ? AND m.status = ?",
			tenantUUID,
			modeliam.SubMember,
			string(coreiam.RoleScopeTenant),
			tenantUUID,
			coreiam.CodeRoleAdmin,
			modeliam.UserStatusActive,
		).
		Order("m.id ASC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, dto.NewInternal("查询租户 active admin 失败", err)
	}
	return row.MemberID, nil
}

func (s *IAMMigrationReportService) writeFixOwnerAudit(ctx context.Context, db *gorm.DB, tenantUUID string, memberID, roleID uint64) error {
	meta, _ := json.Marshal(map[string]any{
		"member_id": memberID,
		"role_id":   roleID,
	})
	after, _ := json.Marshal(map[string]any{
		"role_owner_bound": true,
	})
	actorID := int64(reqctx.GetUserID(ctx))
	event := &modelaudit.AuditEvent{
		OccurredAt:      s.clock().UTC(),
		TenantUUID:      tenantUUID,
		CorrelationID:   reqctx.GetTraceID(ctx),
		Source:          "iam.migration",
		Operation:       "IAM_MIGRATION_FIX_OWNER",
		ResourceType:    "tenant",
		ResourceID:      tenantUUID,
		Outcome:         "SUCCESS",
		Severity:        "INFO",
		ActorUserID:     &actorID,
		ChangesAfter:    datatypes.JSON(after),
		ChangesRedacted: false,
		Meta:            datatypes.JSON(meta),
	}
	if actorID == 0 {
		event.ActorUserID = nil
	}
	if err := db.WithContext(ctx).Create(event).Error; err != nil {
		return dto.NewInternal("写入 IAM 迁移审计失败", err)
	}
	return nil
}

func sortReport(report *IAMMigrationReport) {
	if report == nil {
		return
	}
	sort.Slice(report.RootUsers, func(i, j int) bool { return report.RootUsers[i].ID < report.RootUsers[j].ID })
	sort.Strings(report.TenantOwnerMissing)
	sort.Strings(report.TenantAdminMissing)
	sort.Strings(report.AutoFixCandidates)
	sort.Strings(report.ManualFixRequired)
}
