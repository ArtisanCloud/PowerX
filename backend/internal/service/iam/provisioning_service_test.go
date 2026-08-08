package iam

import (
	"context"
	"strings"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProvisioningServiceCreatesPluginRoleAndMember(t *testing.T) {
	db := setupProvisioningServiceDB(t)
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	svc := NewProvisioningService(db, nil)

	role, err := svc.ProvisionRole(context.Background(), ProvisionRoleInput{
		TenantUUID:  tenantUUID,
		Code:        "plugin_crm_sales",
		Name:        "CRM Sales",
		Description: "Created by plugin provisioning",
	})
	if err != nil {
		t.Fatalf("provision role: %v", err)
	}
	if role.RoleUUID == "" || role.Code != "plugin_crm_sales" {
		t.Fatalf("unexpected role: %#v", role)
	}

	member, err := svc.ProvisionMember(context.Background(), ProvisionMemberInput{
		TenantUUID:      tenantUUID,
		Username:        "alice_crm",
		Email:           "alice@example.com",
		DisplayName:     "Alice",
		InitialPassword: "password123",
		RoleCodes:       []string{"role_user", "plugin_crm_sales"},
	})
	if err != nil {
		t.Fatalf("provision member: %v", err)
	}
	if _, err := uuid.Parse(member.UserUUID); err != nil {
		t.Fatalf("invalid user uuid: %q", member.UserUUID)
	}
	if _, err := uuid.Parse(member.MemberUUID); err != nil {
		t.Fatalf("invalid member uuid: %q", member.MemberUUID)
	}
	if got := strings.Join(member.RoleCodes, ","); got != "plugin_crm_sales,role_user" {
		t.Fatalf("role codes = %q", got)
	}

	var bindings int64
	if err := db.Model(&modeliam.RoleBinding{}).
		Where("tenant_uuid = ? AND subject_uuid = ?", tenantUUID, member.MemberUUID).
		Count(&bindings).Error; err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if bindings != 2 {
		t.Fatalf("bindings = %d, want 2", bindings)
	}
}

func TestProvisioningServiceProvisionMemberIsIdempotentForExistingTenantMember(t *testing.T) {
	db := setupProvisioningServiceDB(t)
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	svc := NewProvisioningService(db, nil)

	first, err := svc.ProvisionMember(context.Background(), ProvisionMemberInput{
		TenantUUID:      tenantUUID,
		Username:        "factory_contact",
		Email:           "factory@example.com",
		DisplayName:     "Factory Contact",
		InitialPassword: "password123",
		RoleCodes:       []string{"role_user"},
	})
	if err != nil {
		t.Fatalf("first provision member: %v", err)
	}
	second, err := svc.ProvisionMember(context.Background(), ProvisionMemberInput{
		TenantUUID:      tenantUUID,
		Username:        "factory_contact_new",
		Email:           "factory@example.com",
		DisplayName:     "Factory Contact New",
		InitialPassword: "password123",
		RoleCodes:       []string{"role_user"},
	})
	if err != nil {
		t.Fatalf("second provision member: %v", err)
	}
	if second.UserUUID != first.UserUUID {
		t.Fatalf("user uuid changed: first=%s second=%s", first.UserUUID, second.UserUUID)
	}
	if second.MemberUUID != first.MemberUUID {
		t.Fatalf("member uuid changed: first=%s second=%s", first.MemberUUID, second.MemberUUID)
	}
	if second.Username != "factory_contact" {
		t.Fatalf("existing member username overwritten or not returned: %q", second.Username)
	}

	var members int64
	if err := db.Model(&modeliam.Member{}).
		Where("tenant_uuid = ? AND user_uuid = ?", tenantUUID, first.UserUUID).
		Count(&members).Error; err != nil {
		t.Fatalf("count members: %v", err)
	}
	if members != 1 {
		t.Fatalf("members = %d, want 1", members)
	}
	var bindings int64
	if err := db.Model(&modeliam.RoleBinding{}).
		Where("tenant_uuid = ? AND subject_uuid = ?", tenantUUID, first.MemberUUID).
		Count(&bindings).Error; err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if bindings != 1 {
		t.Fatalf("bindings = %d, want 1", bindings)
	}
}

func TestProvisioningServiceListsOnlyBindableRoles(t *testing.T) {
	db := setupProvisioningServiceDB(t)
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	svc := NewProvisioningService(db, nil)

	if _, err := svc.ProvisionRole(context.Background(), ProvisionRoleInput{
		TenantUUID: tenantUUID,
		Code:       "plugin_crm_sales",
		Name:       "CRM Sales",
	}); err != nil {
		t.Fatalf("provision plugin role: %v", err)
	}
	if err := db.Create(&modeliam.Role{
		Scope:      string(coreiam.RoleScopeTenant),
		TenantUUID: tenantUUID,
		Code:       coreiam.CodeRoleAdmin,
		Name:       "Tenant Admin",
		Builtin:    true,
	}).Error; err != nil {
		t.Fatalf("seed role_admin: %v", err)
	}

	out, err := svc.ListProvisionRoles(context.Background(), ListProvisionRolesInput{
		TenantUUID:     tenantUUID,
		IncludeBuiltin: true,
		Page:           1,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	got := make([]string, 0, len(out.Items))
	for _, item := range out.Items {
		got = append(got, item.Code)
		if item.RoleUUID == "" {
			t.Fatalf("missing role uuid: %#v", item)
		}
	}
	if strings.Join(got, ",") != "role_user,plugin_crm_sales" {
		t.Fatalf("role codes = %q", strings.Join(got, ","))
	}

	out, err = svc.ListProvisionRoles(context.Background(), ListProvisionRolesInput{
		TenantUUID:     tenantUUID,
		IncludeBuiltin: false,
		Page:           1,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("list plugin roles: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Code != "plugin_crm_sales" {
		t.Fatalf("plugin-only roles = %#v", out.Items)
	}
}

func TestProvisioningServiceRejectsPrivilegedRoleCodes(t *testing.T) {
	db := setupProvisioningServiceDB(t)
	svc := NewProvisioningService(db, nil)

	_, err := svc.ProvisionRole(context.Background(), ProvisionRoleInput{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		Code:       "role_admin",
		Name:       "Tenant Admin",
	})
	if err == nil || !strings.Contains(err.Error(), "role_code_prefix_required") {
		t.Fatalf("expected role code prefix error, got %v", err)
	}

	_, err = svc.ProvisionRole(context.Background(), ProvisionRoleInput{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		Code:       "plugin_",
		Name:       "Plugin",
	})
	if err == nil || !strings.Contains(err.Error(), "role_code_suffix_required") {
		t.Fatalf("expected role code suffix error, got %v", err)
	}

	_, err = svc.ProvisionMember(context.Background(), ProvisionMemberInput{
		TenantUUID:      "6b5d0240-9920-46da-b707-88200e0f51ea",
		Username:        "bob_crm",
		Email:           "bob@example.com",
		DisplayName:     "Bob",
		InitialPassword: "password123",
		RoleCodes:       []string{"role_admin"},
	})
	if err == nil || !strings.Contains(err.Error(), "role_code_not_allowed") {
		t.Fatalf("expected role code not allowed error, got %v", err)
	}
}

func setupProvisioningServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&modeliam.User{},
		&modeliam.Credential{},
		&modeliam.Member{},
		&modeliam.Role{},
		&modeliam.RoleBinding{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	role := modeliam.Role{
		Scope:      string(coreiam.RoleScopeTenant),
		TenantUUID: tenantUUID,
		Code:       coreiam.CodeRoleUser,
		Name:       "Tenant User",
		Builtin:    true,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("seed role_user: %v", err)
	}
	return db
}
