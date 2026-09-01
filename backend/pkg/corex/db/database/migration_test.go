package database

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureIAMUserPhoneIndexDropsLegacyGlobalUniqueIndex(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&modeliam.User{}); err != nil {
		t.Fatalf("automigrate user: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX uk_user_phone ON iam_user (phone)`).Error; err != nil {
		t.Fatalf("create legacy index: %v", err)
	}
	if err := ensureIAMUserPhoneIndex(db); err != nil {
		t.Fatalf("ensure phone index: %v", err)
	}
	for _, user := range []modeliam.User{
		{Email: "empty-phone-a@example.com", DisplayName: "A", Phone: ""},
		{Email: "empty-phone-b@example.com", DisplayName: "B", Phone: ""},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create empty phone user %s: %v", user.Email, err)
		}
	}
}

func TestIAMPhase2UUIDMigrationBackfillsOnce(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&modeliam.Role{}, &modeliam.Permission{}, &modeliam.RolePermission{},
		&modeliam.Member{}, &modeliam.Department{}, &modeliam.MemberDepartment{}, &modeliam.DepartmentClosure{},
	); err != nil {
		t.Fatalf("automigrate legacy IAM tables: %v", err)
	}

	role := modeliam.Role{Scope: "tenant", TenantUUID: "tenant-uuid", Code: "role", Name: "Role"}
	permission := modeliam.Permission{Module: "iam", Resource: "member", Action: "read"}
	member := modeliam.Member{TenantUUID: "tenant-uuid", UserUUID: uuid.NewString(), UserID: 1, Username: "member"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
	parent := modeliam.Department{TenantUUID: "tenant-uuid", Key: "parent", Name: "Parent"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent department: %v", err)
	}
	child := modeliam.Department{TenantUUID: "tenant-uuid", Key: "child", Name: "Child", ParentID: &parent.ID, LeaderMemberID: &member.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child department: %v", err)
	}
	if err := db.Create(&modeliam.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatalf("create role permission: %v", err)
	}
	if err := db.Create(&modeliam.MemberDepartment{MemberID: member.ID, DepartmentID: child.ID, TenantUUID: "tenant-uuid"}).Error; err != nil {
		t.Fatalf("create member department: %v", err)
	}
	if err := db.Create(&modeliam.DepartmentClosure{TenantUUID: "tenant-uuid", AncestorID: parent.ID, DescendantID: child.ID, Depth: 1}).Error; err != nil {
		t.Fatalf("create department closure: %v", err)
	}

	if err := applyIAMPhase2UUIDMigration(db); err != nil {
		t.Fatalf("apply IAM UUID migration: %v", err)
	}
	type departmentUUIDRow struct{ DepartmentUUID, ParentDepartmentUUID, LeaderMemberUUID string }
	var departmentRow departmentUUIDRow
	if err := db.Table((&modeliam.Department{}).GetTableName(true)).Where("id = ?", child.ID).First(&departmentRow).Error; err != nil {
		t.Fatalf("read migrated child department: %v", err)
	}
	if departmentRow.DepartmentUUID == "" || departmentRow.ParentDepartmentUUID == "" || departmentRow.LeaderMemberUUID == "" {
		t.Fatalf("department UUID columns were not fully backfilled: %#v", departmentRow)
	}

	type rolePermissionUUIDRow struct{ RoleUUID, PermissionUUID string }
	var rolePermissionRow rolePermissionUUIDRow
	if err := db.Table((&modeliam.RolePermission{}).GetTableName(true)).First(&rolePermissionRow).Error; err != nil {
		t.Fatalf("read migrated role permission: %v", err)
	}
	if rolePermissionRow.RoleUUID != role.UUID.String() || rolePermissionRow.PermissionUUID == "" {
		t.Fatalf("role permission UUID columns were not backfilled: %#v", rolePermissionRow)
	}

	if err := applyIAMPhase2UUIDMigration(db); err != nil {
		t.Fatalf("reapply IAM UUID migration: %v", err)
	}
	var records int64
	if err := db.Model(&databaseMigrationRecord{}).Where("migration_id = ?", iamPhase2UUIDMigrationID).Count(&records).Error; err != nil {
		t.Fatalf("count migration records: %v", err)
	}
	if records != 1 {
		t.Fatalf("expected exactly one migration record, got %d", records)
	}
}

func TestEnsureIAMRoleBindingTenantMemberIndexRejectsDuplicateDefaultBinding(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&modeliam.RoleBinding{}); err != nil {
		t.Fatalf("automigrate role binding: %v", err)
	}
	if err := ensureIAMRoleBindingTenantMemberIndex(db); err != nil {
		t.Fatalf("ensure role binding index: %v", err)
	}

	binding := modeliam.RoleBinding{
		TenantUUID:  "tenant-uuid",
		RoleUUID:    "role-uuid",
		RoleID:      1,
		SubjectType: modeliam.SubMember,
		SubjectUUID: "member-uuid",
		SubjectID:   2,
		DataScope:   modeliam.ScopeTenant,
	}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatalf("create first role binding: %v", err)
	}
	duplicate := binding
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatalf("expected duplicate tenant member role binding to fail")
	}
}
