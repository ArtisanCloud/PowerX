package iam

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestFixDuplicateRoleBindingsDryRunDoesNotDelete(t *testing.T) {
	db := setupProvisioningServiceDB(t)
	seedDuplicateRoleBindings(t, db)

	result, err := NewIAMMigrationReportService(db).FixDuplicateRoleBindingsAsSystem(context.Background(), false)
	if err != nil {
		t.Fatalf("dry-run duplicate fix: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry-run result")
	}
	if len(result.DuplicateGroups) != 1 {
		t.Fatalf("duplicate groups = %d, want 1", len(result.DuplicateGroups))
	}

	var count int64
	if err := db.Model(&modeliam.RoleBinding{}).Count(&count).Error; err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if count != 3 {
		t.Fatalf("role bindings = %d, want 3", count)
	}
}

func TestFixDuplicateRoleBindingsDeletesDuplicates(t *testing.T) {
	db := setupProvisioningServiceDB(t)
	seedDuplicateRoleBindings(t, db)

	result, err := NewIAMMigrationReportService(db).FixDuplicateRoleBindingsAsSystem(context.Background(), true)
	if err != nil {
		t.Fatalf("fix duplicate role bindings: %v", err)
	}
	if result.DryRun {
		t.Fatal("expected confirmed fix result")
	}
	if len(result.KeptIDs) != 1 || result.KeptIDs[0] != 1 {
		t.Fatalf("kept ids = %#v, want [1]", result.KeptIDs)
	}
	if len(result.DeletedIDs) != 2 {
		t.Fatalf("deleted ids = %#v, want 2 rows", result.DeletedIDs)
	}
	if len(result.RemainingDuplicateGroups) != 0 {
		t.Fatalf("remaining duplicate groups = %d, want 0", len(result.RemainingDuplicateGroups))
	}

	var rows []modeliam.RoleBinding
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("list role bindings: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("remaining rows = %#v, want only id=1", rows)
	}
}

func seedDuplicateRoleBindings(t *testing.T, db *gorm.DB) {
	t.Helper()
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	user := modeliam.User{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
		Email:          "duplicate@example.com",
		DisplayName:    "Duplicate Binding User",
		Status:         modeliam.UserStatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	member := modeliam.Member{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
		TenantUUID:     tenantUUID,
		UserUUID:       user.UUID.String(),
		UserID:         user.ID,
		Username:       "duplicate_binding_user",
		Status:         modeliam.UserStatusActive,
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
	var role modeliam.Role
	if err := db.Where("tenant_uuid = ? AND code = ?", tenantUUID, "role_user").Take(&role).Error; err != nil {
		t.Fatalf("load role: %v", err)
	}
	if role.UUID == uuid.Nil {
		role.UUID = uuid.New()
		if err := db.Model(&role).Update("uuid", role.UUID).Error; err != nil {
			t.Fatalf("backfill role uuid: %v", err)
		}
	}
	rows := []modeliam.RoleBinding{
		{
			TenantUUID:  tenantUUID,
			RoleUUID:    role.UUID.String(),
			RoleID:      role.ID,
			SubjectType: modeliam.SubMember,
			SubjectUUID: member.UUID.String(),
			SubjectID:   member.ID,
			DataScope:   modeliam.ScopeTenant,
		},
		{
			TenantUUID:  tenantUUID,
			RoleUUID:    role.UUID.String(),
			RoleID:      role.ID,
			SubjectType: modeliam.SubMember,
			SubjectUUID: member.UUID.String(),
			SubjectID:   member.ID,
			DataScope:   modeliam.ScopeTenant,
		},
		{
			TenantUUID:  tenantUUID,
			RoleUUID:    role.UUID.String(),
			RoleID:      role.ID,
			SubjectType: modeliam.SubMember,
			SubjectUUID: member.UUID.String(),
			SubjectID:   member.ID,
			DataScope:   modeliam.ScopeTenant,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed duplicate bindings: %v", err)
	}
}
