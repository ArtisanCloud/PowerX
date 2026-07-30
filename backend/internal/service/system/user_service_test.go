package system

import (
	"context"
	"errors"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeTenantUsername(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "trims and lowercases", raw: " Alice.Dev ", want: "alice.dev"},
		{name: "allows underscore and hyphen", raw: "ops_user-01", want: "ops_user-01"},
		{name: "rejects empty", raw: " ", wantErr: true},
		{name: "rejects leading digit", raw: "1alice", wantErr: true},
		{name: "rejects too short", raw: "ab", wantErr: true},
		{name: "rejects uppercase after normalization if invalid characters remain", raw: "Alice Dev", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeTenantUsername(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got username %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSyncPasswordCredentialIdentifierTx(t *testing.T) {
	db := newUserServiceTestDB(t)
	ctx := context.Background()
	if err := db.Create(&m.Credential{
		UserID:     1,
		Provider:   "password",
		Identifier: "old@example.com",
		SecretHash: "hash",
		IsPrimary:  true,
	}).Error; err != nil {
		t.Fatalf("create old credential: %v", err)
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return syncPasswordCredentialIdentifierTx(ctx, tx, 1, "old@example.com", "new@example.com")
	})
	if err != nil {
		t.Fatalf("sync credential: %v", err)
	}

	var oldCount int64
	if err := db.Model(&m.Credential{}).
		Where("provider = ? AND identifier = ?", "password", "old@example.com").
		Count(&oldCount).Error; err != nil {
		t.Fatalf("count old credential: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("expected old credential removed, got %d", oldCount)
	}
	var next m.Credential
	if err := db.Where("provider = ? AND identifier = ?", "password", "new@example.com").First(&next).Error; err != nil {
		t.Fatalf("find new credential: %v", err)
	}
	if next.UserID != 1 || next.SecretHash != "hash" {
		t.Fatalf("unexpected new credential: %+v", next)
	}
}

func TestSyncPasswordCredentialIdentifierTxRejectsTakenIdentifier(t *testing.T) {
	db := newUserServiceTestDB(t)
	ctx := context.Background()
	if err := db.Create(&[]m.Credential{
		{UserID: 1, Provider: "password", Identifier: "old@example.com", SecretHash: "hash", IsPrimary: true},
		{UserID: 2, Provider: "password", Identifier: "taken@example.com", SecretHash: "hash", IsPrimary: true},
	}).Error; err != nil {
		t.Fatalf("create credentials: %v", err)
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return syncPasswordCredentialIdentifierTx(ctx, tx, 1, "old@example.com", "taken@example.com")
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestCreateSystemUserAppliesExplicitTenantRoleUUIDs(t *testing.T) {
	db := newUserServiceTestDB(t)
	ctx := context.Background()
	tenantUUID := uuid.NewString()
	role := m.Role{
		Scope:      "tenant",
		TenantUUID: tenantUUID,
		Code:       "role_vendor",
		Name:       "Vendor",
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	svc := NewUserService(db)
	userUUID, err := svc.CreateSystemUser(ctx, &m.User{
		Email:       "vendor@example.com",
		DisplayName: "Vendor User",
		Status:      1,
	}, tenantUUID, "vendor_user", "", nil, []string{role.UUID.String()})
	if err != nil {
		t.Fatalf("create system user: %v", err)
	}
	userID, err := svc.ResolveUserIDByUUID(ctx, userUUID)
	if err != nil {
		t.Fatalf("resolve user uuid: %v", err)
	}

	roleUUIDs, err := svc.ListUserRoleUUIDs(ctx, userID, tenantUUID)
	if err != nil {
		t.Fatalf("list user role uuids: %v", err)
	}
	if len(roleUUIDs) != 1 || roleUUIDs[0] != role.UUID.String() {
		t.Fatalf("expected role %s, got %+v", role.UUID.String(), roleUUIDs)
	}

	var member m.Member
	if err := db.Where("tenant_uuid = ? AND user_uuid = ?", tenantUUID, userUUID).First(&member).Error; err != nil {
		t.Fatalf("expected member user_uuid to be persisted: %v", err)
	}
	var binding m.RoleBinding
	if err := db.Where("tenant_uuid = ? AND role_uuid = ? AND subject_uuid = ?", tenantUUID, role.UUID.String(), member.UUID.String()).First(&binding).Error; err != nil {
		t.Fatalf("expected role binding uuids to be persisted: %v", err)
	}
}

func newUserServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = originalSchema
	})

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&m.User{}, &m.Member{}, &m.Role{}, &m.RoleBinding{}, &m.Credential{}); err != nil {
		t.Fatalf("migrate user service tables: %v", err)
	}
	return db
}

func TestSyncPasswordCredentialIdentifierTxIgnoresMissingOldCredential(t *testing.T) {
	db := newUserServiceTestDB(t)
	ctx := context.Background()

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return syncPasswordCredentialIdentifierTx(ctx, tx, 1, "missing@example.com", "new@example.com")
	})
	if err != nil {
		t.Fatalf("expected missing old credential to be ignored, got %v", err)
	}

	var count int64
	if err := db.Model(&m.Credential{}).Count(&count).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("count credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no credential to be created, got %d", count)
	}
}
