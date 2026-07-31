package database

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
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
