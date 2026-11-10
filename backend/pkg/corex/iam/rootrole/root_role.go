// Package rootrole 维护 Core 内置 root 角色；它不是可人工配置的业务角色。
package rootrole

import (
	"errors"
	"strings"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrManaged = errors.New("IAM_ROOT_ROLE_CORE_MANAGED")
var ErrCatalogInconsistent = errors.New("IAM_ROOT_ROLE_CATALOG_INCONSISTENT")

// IsManaged 不依赖可变的 builtin 标记，避免旧数据丢失标记后开放编辑。
func IsManaged(role im.Role) bool {
	return role.Scope == string(iam.RoleScopeSystem) && strings.TrimSpace(role.TenantUUID) == "" && role.Code == iam.CodeSystemRoot
}

// Ensure 在生命周期或授权源事务内维护既有 root 角色及正式目录权限。
// 保留 UUID/ID，不创建租户成员、不修改 role_admin，也不依赖插件名称。
func Ensure(db *gorm.DB) (bool, error) {
	ready, err := schemaReady(db)
	if err != nil || !ready {
		return false, err
	}
	var role im.Role
	err = db.Unscoped().Where("scope = ? AND tenant_uuid = ? AND code = ?", iam.RoleScopeSystem, "", iam.CodeSystemRoot).Take(&role).Error
	changed := false
	if errors.Is(err, gorm.ErrRecordNotFound) {
		role = im.Role{Scope: string(iam.RoleScopeSystem), Code: iam.CodeSystemRoot, Name: "Super Admin", Builtin: true}
		if err = db.Create(&role).Error; err != nil {
			return false, err
		}
		changed = true
	} else if err != nil {
		return false, err
	}
	// 数字主键冲突不代表 UUID 关系正确。旧关系指向其他 UUID 时，不能
	// 通过 ON CONFLICT DO NOTHING 冒充同步成功，也不在启动/安装中修旧数据。
	if err = checkRoleCatalog(db, role); err != nil {
		return changed, err
	}
	if !role.Builtin || role.DeletedAt.Valid {
		if err = db.Unscoped().Model(&role).Updates(map[string]any{"builtin": true, "deleted_at": nil}).Error; err != nil {
			return false, err
		}
		changed = true
	}
	var permissions []im.Permission
	err = db.Table((&im.Permission{}).TableName()+" AS p").Select("p.id, p.permission_uuid").
		Where("p.status = ? AND p.deleted_at IS NULL", im.PermissionStatusActive).
		Where("NOT EXISTS (SELECT 1 FROM "+(&im.RolePermission{}).TableName()+" rp WHERE rp.role_uuid = ? AND rp.permission_uuid = p.permission_uuid)", role.UUID).
		Find(&permissions).Error
	if err != nil || len(permissions) == 0 {
		return changed, err
	}
	grants := make([]im.RolePermission, 0, len(permissions))
	for _, permission := range permissions {
		grants = append(grants, im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID})
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(grants, 200)
	if result.Error != nil {
		return changed, result.Error
	}
	missing, err := missingCatalogPermissions(db, role)
	if err == nil && missing > 0 {
		err = ErrCatalogInconsistent
	}
	return changed || result.RowsAffected > 0, err
}

// CheckCatalog 只读诊断 Root 关联；不重绑历史 UUID，也不写授权源。
func CheckCatalog(db *gorm.DB) error {
	ready, err := schemaReady(db)
	if err != nil || !ready {
		return err
	}
	var role im.Role
	if err := db.Unscoped().Where("scope=? AND tenant_uuid=? AND code=?", iam.RoleScopeSystem, "", iam.CodeSystemRoot).Take(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCatalogInconsistent
		}
		return err
	}
	if !role.Builtin || role.DeletedAt.Valid || role.UUID == uuid.Nil {
		return ErrCatalogInconsistent
	}
	if err := checkRoleCatalog(db, role); err != nil {
		return err
	}
	missing, err := missingCatalogPermissions(db, role)
	if err == nil && missing > 0 {
		return ErrCatalogInconsistent
	}
	return err
}

func missingCatalogPermissions(db *gorm.DB, role im.Role) (int64, error) {
	var missing int64
	err := db.Table((&im.Permission{}).TableName()+" p").Where("p.status=? AND p.deleted_at IS NULL", im.PermissionStatusActive).
		Where("NOT EXISTS (SELECT 1 FROM "+(&im.RolePermission{}).TableName()+" rp WHERE rp.role_uuid=? AND rp.permission_uuid=p.permission_uuid)", role.UUID).Count(&missing).Error
	return missing, err
}

func checkRoleCatalog(db *gorm.DB, role im.Role) error {
	var inconsistent int64
	err := db.Table((&im.RolePermission{}).TableName()+" rp").
		Joins("LEFT JOIN "+(&im.Permission{}).TableName()+" p ON p.id=rp.permission_id").
		Where("(rp.role_id=? OR rp.role_uuid=?) AND (rp.role_id<>? OR rp.role_uuid IS NULL OR rp.role_uuid<>? OR p.id IS NULL OR rp.permission_uuid IS NULL OR rp.permission_uuid<>p.permission_uuid)", role.ID, role.UUID, role.ID, role.UUID).
		Count(&inconsistent).Error
	if err == nil && inconsistent > 0 {
		return ErrCatalogInconsistent
	}
	return err
}

func schemaReady(db *gorm.DB) (bool, error) {
	// 正式迁移先创建旧结构，再补 UUID 关系；中途不能使用尚未迁移的列。
	// 迁移完成后的统一生命周期入口会再次执行，避免首次安装半途失败。
	if db.Dialector.Name() == "postgres" {
		// 在每个源事务调用，合并成一次轻量目录查询，避免逐列加载完整列元数据。
		var ready bool
		err := db.Raw(`SELECT COUNT(*)=4 FROM pg_catalog.pg_attribute WHERE NOT attisdropped AND (
		 (attrelid=to_regclass(?) AND attname='uuid') OR
		 (attrelid=to_regclass(?) AND attname='permission_uuid') OR
		 (attrelid=to_regclass(?) AND attname IN ('role_uuid','permission_uuid')))`,
			(&im.Role{}).TableName(), (&im.Permission{}).TableName(), (&im.RolePermission{}).TableName()).Scan(&ready).Error
		return ready, err
	}
	for _, model := range []any{&im.Role{}, &im.Permission{}, &im.RolePermission{}} {
		if !db.Migrator().HasTable(model) {
			return false, nil
		}
	}
	for _, column := range []struct {
		model any
		name  string
	}{
		{&im.Role{}, "uuid"}, {&im.Permission{}, "permission_uuid"},
		{&im.RolePermission{}, "role_uuid"}, {&im.RolePermission{}, "permission_uuid"},
	} {
		if !db.Migrator().HasColumn(column.model, column.name) {
			return false, nil
		}
	}
	return true, nil
}
