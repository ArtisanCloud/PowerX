package database

import (
	"errors"
	"fmt"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	iamPhase2UUIDMigrationID          = "202608310001_iam_phase2_uuid_relations"
	iamPhase2UUIDIntegrityMigrationID = "202608310002_iam_phase2_uuid_integrity"
)

// databaseMigrationRecord records completed, data-bearing schema migrations.
// It is deliberately separate from business tables: a migration is committed
// only after its schema changes, backfill, and validation all succeed.
type databaseMigrationRecord struct {
	MigrationID string    `gorm:"column:migration_id;type:varchar(128);primaryKey"`
	AppliedAt   time.Time `gorm:"column:applied_at;not null"`
}

func (databaseMigrationRecord) TableName() string {
	return coremodel.PowerXSchema + ".database_migration_records"
}

// Phase 2 shadow models are intentionally migration-only. Keeping UUID columns
// out of the runtime models until this migration has completed prevents
// AutoMigrate from attempting to rewrite legacy primary keys.
type iamPermissionUUIDColumns struct {
	PermissionUUID string `gorm:"column:permission_uuid;type:uuid;uniqueIndex:uk_iam_permission_permission_uuid"`
}

func (iamPermissionUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMPermission
}

type iamDepartmentUUIDColumns struct {
	DepartmentUUID       string `gorm:"column:department_uuid;type:uuid;uniqueIndex:uk_iam_department_department_uuid"`
	ParentDepartmentUUID string `gorm:"column:parent_department_uuid;type:uuid;index"`
	LeaderMemberUUID     string `gorm:"column:leader_member_uuid;type:uuid;index"`
}

func (iamDepartmentUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMDepartment
}

type iamRolePermissionUUIDColumns struct {
	RoleUUID       string `gorm:"column:role_uuid;type:uuid;uniqueIndex:uk_iam_role_permission_role_permission_uuid"`
	PermissionUUID string `gorm:"column:permission_uuid;type:uuid;uniqueIndex:uk_iam_role_permission_role_permission_uuid"`
}

func (iamRolePermissionUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMRolePermission
}

type iamMemberDepartmentUUIDColumns struct {
	MemberUUID     string `gorm:"column:member_uuid;type:uuid;uniqueIndex:uk_iam_member_department_member_department_uuid"`
	DepartmentUUID string `gorm:"column:department_uuid;type:uuid;uniqueIndex:uk_iam_member_department_member_department_uuid"`
}

func (iamMemberDepartmentUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMMemberDepartment
}

type iamDepartmentClosureUUIDColumns struct {
	AncestorDepartmentUUID   string `gorm:"column:ancestor_department_uuid;type:uuid;uniqueIndex:uk_iam_department_closure_uuid"`
	DescendantDepartmentUUID string `gorm:"column:descendant_department_uuid;type:uuid;uniqueIndex:uk_iam_department_closure_uuid"`
	TenantUUID               string `gorm:"column:tenant_uuid;uniqueIndex:uk_iam_department_closure_uuid"`
}

func (iamDepartmentClosureUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMDepartmentClosure
}

func applyIAMPhase2UUIDMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if err := db.AutoMigrate(&databaseMigrationRecord{}); err != nil {
		return fmt.Errorf("create migration journal failed: %w", err)
	}

	var applied databaseMigrationRecord
	err := db.Where("migration_id = ?", iamPhase2UUIDMigrationID).First(&applied).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read migration journal failed: %w", err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var record databaseMigrationRecord
		err := tx.Where("migration_id = ?", iamPhase2UUIDMigrationID).First(&record).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lock migration journal failed: %w", err)
		}

		if err := addIAMPhase2UUIDColumns(tx); err != nil {
			return err
		}
		if err := backfillIAMPhase2UUIDColumns(tx); err != nil {
			return err
		}
		if err := validateIAMPhase2UUIDColumns(tx); err != nil {
			return err
		}
		if err := createIAMPhase2UUIDIndexes(tx); err != nil {
			return err
		}
		if err := tx.Create(&databaseMigrationRecord{MigrationID: iamPhase2UUIDMigrationID, AppliedAt: time.Now().UTC()}).Error; err != nil {
			return fmt.Errorf("record completed IAM phase 2 UUID migration failed: %w", err)
		}
		return nil
	})
}

// applyIAMPhase2UUIDIntegrityMigration repairs rows inserted after the initial
// one-time backfill but before UUID write hooks were introduced. It is itself a
// versioned one-time migration; all subsequent writes are maintained by model
// hooks and must fail instead of persisting an unresolved UUID relationship.
func applyIAMPhase2UUIDIntegrityMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	var applied databaseMigrationRecord
	err := db.Where("migration_id = ?", iamPhase2UUIDIntegrityMigrationID).First(&applied).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read IAM UUID integrity migration journal failed: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var record databaseMigrationRecord
		err := tx.Where("migration_id = ?", iamPhase2UUIDIntegrityMigrationID).First(&record).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lock IAM UUID integrity migration journal failed: %w", err)
		}
		if err := backfillIAMPhase2UUIDColumns(tx); err != nil {
			return err
		}
		if err := validateIAMPhase2UUIDColumns(tx); err != nil {
			return err
		}
		if err := tx.Create(&databaseMigrationRecord{MigrationID: iamPhase2UUIDIntegrityMigrationID, AppliedAt: time.Now().UTC()}).Error; err != nil {
			return fmt.Errorf("record IAM UUID integrity migration failed: %w", err)
		}
		return nil
	})
}

func addIAMPhase2UUIDColumns(db *gorm.DB) error {
	for _, column := range []struct {
		model  any
		column string
	}{
		{iamPermissionUUIDColumns{}, "PermissionUUID"},
		{iamDepartmentUUIDColumns{}, "DepartmentUUID"},
		{iamDepartmentUUIDColumns{}, "ParentDepartmentUUID"},
		{iamDepartmentUUIDColumns{}, "LeaderMemberUUID"},
		{iamRolePermissionUUIDColumns{}, "RoleUUID"},
		{iamRolePermissionUUIDColumns{}, "PermissionUUID"},
		{iamMemberDepartmentUUIDColumns{}, "MemberUUID"},
		{iamMemberDepartmentUUIDColumns{}, "DepartmentUUID"},
		{iamDepartmentClosureUUIDColumns{}, "AncestorDepartmentUUID"},
		{iamDepartmentClosureUUIDColumns{}, "DescendantDepartmentUUID"},
	} {
		if db.Migrator().HasColumn(column.model, column.column) {
			continue
		}
		if err := db.Migrator().AddColumn(column.model, column.column); err != nil && !db.Migrator().HasColumn(column.model, column.column) {
			return fmt.Errorf("add IAM phase 2 UUID column %s failed: %w", column.column, err)
		}
	}
	return nil
}

func createIAMPhase2UUIDIndexes(db *gorm.DB) error {
	for _, index := range []struct {
		model any
		name  string
	}{
		{iamPermissionUUIDColumns{}, "uk_iam_permission_permission_uuid"},
		{iamDepartmentUUIDColumns{}, "uk_iam_department_department_uuid"},
		{iamRolePermissionUUIDColumns{}, "uk_iam_role_permission_role_permission_uuid"},
		{iamMemberDepartmentUUIDColumns{}, "uk_iam_member_department_member_department_uuid"},
		{iamDepartmentClosureUUIDColumns{}, "uk_iam_department_closure_uuid"},
	} {
		if db.Migrator().HasIndex(index.model, index.name) {
			continue
		}
		if err := db.Migrator().CreateIndex(index.model, index.name); err != nil {
			return fmt.Errorf("create IAM phase 2 UUID index %s failed: %w", index.name, err)
		}
	}
	return nil
}

func backfillIAMPhase2UUIDColumns(db *gorm.DB) error {
	permissionUUIDs, err := backfillPermissionUUIDs(db)
	if err != nil {
		return err
	}
	departmentUUIDs, err := backfillDepartmentUUIDs(db)
	if err != nil {
		return err
	}
	memberUUIDs, err := iamMemberUUIDMap(db)
	if err != nil {
		return err
	}
	roleUUIDs, err := iamRoleUUIDMap(db)
	if err != nil {
		return err
	}
	if err := backfillDepartmentRelations(db, departmentUUIDs, memberUUIDs); err != nil {
		return err
	}
	if err := backfillRolePermissions(db, roleUUIDs, permissionUUIDs); err != nil {
		return err
	}
	if err := backfillMemberDepartments(db, memberUUIDs, departmentUUIDs); err != nil {
		return err
	}
	return backfillDepartmentClosures(db, departmentUUIDs)
}

type iamIDUUIDRow struct {
	ID   uint64    `gorm:"column:id"`
	UUID uuid.UUID `gorm:"column:uuid"`
}

func iamRoleUUIDMap(db *gorm.DB) (map[uint64]string, error) {
	var rows []iamIDUUIDRow
	if err := db.Table((&modelIAM.Role{}).GetTableName(true)).Select("id, uuid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list IAM roles for UUID backfill failed: %w", err)
	}
	result := make(map[uint64]string, len(rows))
	for _, row := range rows {
		if row.ID == 0 || row.UUID == uuid.Nil {
			return nil, fmt.Errorf("IAM role id %d has no UUID", row.ID)
		}
		result[row.ID] = row.UUID.String()
	}
	return result, nil
}

func iamMemberUUIDMap(db *gorm.DB) (map[uint64]string, error) {
	var rows []iamIDUUIDRow
	if err := db.Table((&modelIAM.Member{}).GetTableName(true)).Select("id, uuid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list IAM members for UUID backfill failed: %w", err)
	}
	result := make(map[uint64]string, len(rows))
	for _, row := range rows {
		if row.ID == 0 || row.UUID == uuid.Nil {
			return nil, fmt.Errorf("IAM member id %d has no UUID", row.ID)
		}
		result[row.ID] = row.UUID.String()
	}
	return result, nil
}

func backfillPermissionUUIDs(db *gorm.DB) (map[uint64]string, error) {
	table := (&modelIAM.Permission{}).GetTableName(true)
	type row struct {
		ID             uint64 `gorm:"column:id"`
		PermissionUUID string `gorm:"column:permission_uuid"`
	}
	var rows []row
	if err := db.Table(table).Select("id, permission_uuid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list IAM permissions for UUID backfill failed: %w", err)
	}
	result := make(map[uint64]string, len(rows))
	for _, row := range rows {
		value := row.PermissionUUID
		if value == "" {
			value = uuid.NewString()
			if err := db.Table(table).Where("id = ?", row.ID).Update("permission_uuid", value).Error; err != nil {
				return nil, fmt.Errorf("backfill IAM permission id %d UUID failed: %w", row.ID, err)
			}
		}
		result[row.ID] = value
	}
	return result, nil
}

func backfillDepartmentUUIDs(db *gorm.DB) (map[uint64]string, error) {
	table := (&modelIAM.Department{}).GetTableName(true)
	type row struct {
		ID             uint64 `gorm:"column:id"`
		DepartmentUUID string `gorm:"column:department_uuid"`
	}
	var rows []row
	if err := db.Table(table).Select("id, department_uuid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list IAM departments for UUID backfill failed: %w", err)
	}
	result := make(map[uint64]string, len(rows))
	for _, row := range rows {
		value := row.DepartmentUUID
		if value == "" {
			value = uuid.NewString()
			if err := db.Table(table).Where("id = ?", row.ID).Update("department_uuid", value).Error; err != nil {
				return nil, fmt.Errorf("backfill IAM department id %d UUID failed: %w", row.ID, err)
			}
		}
		result[row.ID] = value
	}
	return result, nil
}

func backfillDepartmentRelations(db *gorm.DB, departments, members map[uint64]string) error {
	table := (&modelIAM.Department{}).GetTableName(true)
	type row struct {
		ID             uint64  `gorm:"column:id"`
		ParentID       *uint64 `gorm:"column:parent_id"`
		LeaderMemberID *uint64 `gorm:"column:leader_member_id"`
	}
	var rows []row
	if err := db.Table(table).Select("id, parent_id, leader_member_id").Find(&rows).Error; err != nil {
		return fmt.Errorf("list IAM department relations failed: %w", err)
	}
	for _, row := range rows {
		updates := map[string]any{}
		if row.ParentID != nil {
			value, ok := departments[*row.ParentID]
			if !ok {
				return fmt.Errorf("IAM department id %d references missing parent id %d", row.ID, *row.ParentID)
			}
			updates["parent_department_uuid"] = value
		}
		if row.LeaderMemberID != nil {
			value, ok := members[*row.LeaderMemberID]
			if !ok {
				return fmt.Errorf("IAM department id %d references missing leader member id %d", row.ID, *row.LeaderMemberID)
			}
			updates["leader_member_uuid"] = value
		}
		if len(updates) > 0 {
			if err := db.Table(table).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("backfill IAM department id %d relations failed: %w", row.ID, err)
			}
		}
	}
	return nil
}

func backfillRolePermissions(db *gorm.DB, roles, permissions map[uint64]string) error {
	table := (&modelIAM.RolePermission{}).GetTableName(true)
	type row struct {
		RoleID       uint64 `gorm:"column:role_id"`
		PermissionID uint64 `gorm:"column:permission_id"`
	}
	var rows []row
	if err := db.Table(table).Select("role_id, permission_id").Find(&rows).Error; err != nil {
		return fmt.Errorf("list IAM role permissions failed: %w", err)
	}
	for _, row := range rows {
		roleUUID, ok := roles[row.RoleID]
		if !ok {
			return fmt.Errorf("IAM role permission references missing role id %d", row.RoleID)
		}
		permissionUUID, ok := permissions[row.PermissionID]
		if !ok {
			return fmt.Errorf("IAM role permission references missing permission id %d", row.PermissionID)
		}
		if err := db.Table(table).Where("role_id = ? AND permission_id = ?", row.RoleID, row.PermissionID).
			Updates(map[string]any{"role_uuid": roleUUID, "permission_uuid": permissionUUID}).Error; err != nil {
			return fmt.Errorf("backfill IAM role permission (%d, %d) failed: %w", row.RoleID, row.PermissionID, err)
		}
	}
	return nil
}

func backfillMemberDepartments(db *gorm.DB, members, departments map[uint64]string) error {
	table := (&modelIAM.MemberDepartment{}).GetTableName(true)
	type row struct {
		MemberID     uint64 `gorm:"column:member_id"`
		DepartmentID uint64 `gorm:"column:department_id"`
	}
	var rows []row
	if err := db.Table(table).Select("member_id, department_id").Find(&rows).Error; err != nil {
		return fmt.Errorf("list IAM member departments failed: %w", err)
	}
	for _, row := range rows {
		memberUUID, ok := members[row.MemberID]
		if !ok {
			return fmt.Errorf("IAM member department references missing member id %d", row.MemberID)
		}
		departmentUUID, ok := departments[row.DepartmentID]
		if !ok {
			return fmt.Errorf("IAM member department references missing department id %d", row.DepartmentID)
		}
		if err := db.Table(table).Where("member_id = ? AND department_id = ?", row.MemberID, row.DepartmentID).
			Updates(map[string]any{"member_uuid": memberUUID, "department_uuid": departmentUUID}).Error; err != nil {
			return fmt.Errorf("backfill IAM member department (%d, %d) failed: %w", row.MemberID, row.DepartmentID, err)
		}
	}
	return nil
}

func backfillDepartmentClosures(db *gorm.DB, departments map[uint64]string) error {
	table := (&modelIAM.DepartmentClosure{}).GetTableName(true)
	type row struct {
		TenantUUID   string `gorm:"column:tenant_uuid"`
		AncestorID   uint64 `gorm:"column:ancestor_id"`
		DescendantID uint64 `gorm:"column:descendant_id"`
	}
	var rows []row
	if err := db.Table(table).Select("tenant_uuid, ancestor_id, descendant_id").Find(&rows).Error; err != nil {
		return fmt.Errorf("list IAM department closures failed: %w", err)
	}
	for _, row := range rows {
		ancestorUUID, ok := departments[row.AncestorID]
		if !ok {
			return fmt.Errorf("IAM department closure references missing ancestor id %d", row.AncestorID)
		}
		descendantUUID, ok := departments[row.DescendantID]
		if !ok {
			return fmt.Errorf("IAM department closure references missing descendant id %d", row.DescendantID)
		}
		if err := db.Table(table).Where("tenant_uuid = ? AND ancestor_id = ? AND descendant_id = ?", row.TenantUUID, row.AncestorID, row.DescendantID).
			Updates(map[string]any{"ancestor_department_uuid": ancestorUUID, "descendant_department_uuid": descendantUUID}).Error; err != nil {
			return fmt.Errorf("backfill IAM department closure (%d, %d) failed: %w", row.AncestorID, row.DescendantID, err)
		}
	}
	return nil
}

func validateIAMPhase2UUIDColumns(db *gorm.DB) error {
	for _, check := range []struct {
		table     string
		condition string
		field     string
	}{
		{(&modelIAM.Permission{}).GetTableName(true), "permission_uuid IS NULL", "permission_uuid"},
		{(&modelIAM.Department{}).GetTableName(true), "department_uuid IS NULL", "department_uuid"},
		{(&modelIAM.Department{}).GetTableName(true), "parent_id IS NOT NULL AND parent_department_uuid IS NULL", "parent_department_uuid"},
		{(&modelIAM.Department{}).GetTableName(true), "leader_member_id IS NOT NULL AND leader_member_uuid IS NULL", "leader_member_uuid"},
		{(&modelIAM.RolePermission{}).GetTableName(true), "role_uuid IS NULL", "role_uuid"},
		{(&modelIAM.RolePermission{}).GetTableName(true), "permission_uuid IS NULL", "permission_uuid"},
		{(&modelIAM.MemberDepartment{}).GetTableName(true), "member_uuid IS NULL", "member_uuid"},
		{(&modelIAM.MemberDepartment{}).GetTableName(true), "department_uuid IS NULL", "department_uuid"},
		{(&modelIAM.DepartmentClosure{}).GetTableName(true), "ancestor_department_uuid IS NULL", "ancestor_department_uuid"},
		{(&modelIAM.DepartmentClosure{}).GetTableName(true), "descendant_department_uuid IS NULL", "descendant_department_uuid"},
	} {
		var count int64
		if err := db.Table(check.table).Where(check.condition).Count(&count).Error; err != nil {
			return fmt.Errorf("validate IAM phase 2 UUID column %s failed: %w", check.field, err)
		}
		if count != 0 {
			return fmt.Errorf("IAM phase 2 UUID migration left %d row(s) without %s", count, check.field)
		}
	}
	return nil
}
