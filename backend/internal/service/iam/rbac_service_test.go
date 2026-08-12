package iam

import (
	"context"
	"encoding/json"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetPermissionIDsExpandsPluginMenuToMatchingPageRead(t *testing.T) {
	db := setupRBACServiceTestDB(t)
	ctx := context.Background()
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	role := dbm.Role{
		Scope:      string(coreiam.RoleScopeTenant),
		TenantUUID: tenantUUID,
		Code:       coreiam.CodeRoleAdmin,
		Name:       "Tenant Admin",
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	menu := dbm.Permission{
		Module: "menu", Resource: "demo", Action: "view",
		Effect: "allow", Status: dbm.PermissionStatusActive, Source: "plugin:com.powerx.plugins.demo",
		Meta: mustJSON(t, map[string]any{
			"type": "menu", "origin": "plugin", "plugin_id": "com.powerx.plugins.demo",
			"permission": "menu.demo:view", "menu_path": []string{"demo"}, "page_permission_codes": []string{"operations.demo:read"},
		}),
	}
	page := dbm.Permission{
		Module: "operations", Resource: "demo", Action: "read",
		Effect: "allow", Status: dbm.PermissionStatusActive, Source: "plugin:com.powerx.plugins.demo",
		Meta: mustJSON(t, map[string]any{
			"type": "page", "permission": "operations.demo:read",
			"protocol_bindings": []map[string]any{{
				"channel": "rest", "method": "GET", "path": "/admin/operations/demo",
			}},
		}),
	}
	action := dbm.Permission{
		Module: "operations", Resource: "demo", Action: "manage",
		Effect: "allow", Status: dbm.PermissionStatusActive, Source: "plugin:com.powerx.plugins.demo",
		Meta: mustJSON(t, map[string]any{"type": "action", "permission": "operations.demo:manage"}),
	}
	if err := db.Create(&[]dbm.Permission{menu, page, action}).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	var rows []dbm.Permission
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		t.Fatalf("list permissions: %v", err)
	}

	svc := NewRBACService(db)
	res, err := svc.SetPermissionIDs(ctx, ActorContext{IsRoot: true}, role.ID, []uint64{rows[0].ID})
	if err != nil {
		t.Fatalf("SetPermissionIDs() err = %v", err)
	}
	got := map[uint64]struct{}{}
	for _, id := range res.Now {
		got[id] = struct{}{}
	}
	if _, ok := got[rows[0].ID]; !ok {
		t.Fatalf("menu permission not granted: now=%v", res.Now)
	}
	if _, ok := got[rows[1].ID]; !ok {
		t.Fatalf("matching page permission not expanded: now=%v", res.Now)
	}
	if _, ok := got[rows[2].ID]; ok {
		t.Fatalf("action permission must not be expanded from menu: now=%v", res.Now)
	}
}

func setupRBACServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&dbm.Role{}, &dbm.Permission{}, &dbm.RolePermission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func mustJSON(t *testing.T, value any) datatypes.JSON {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}
