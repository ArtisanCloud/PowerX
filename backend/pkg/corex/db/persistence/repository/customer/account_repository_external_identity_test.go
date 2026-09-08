package customer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveOrCreateExternalIdentityIsScopedToProviderPlugin(t *testing.T) {
	db := newExternalIdentityRepositoryDB(t)
	repo := NewAccountRepository(db)
	ctx := context.Background()
	providerA, err := ExternalIdentityProviderKey("com.powerx.plugins.channel-a")
	require.NoError(t, err)
	providerB, err := ExternalIdentityProviderKey("com.powerx.plugins.channel-b")
	require.NoError(t, err)

	first, err := repo.ResolveOrCreateExternalIdentity(ctx, ExternalIdentityInput{
		TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: providerA, ProviderPluginID: "com.powerx.plugins.channel-a",
		ProviderSubject: "channel-user-1", DisplayName: "客户甲",
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.CustomerUUID)
	require.Equal(t, "客户甲", first.DisplayName)

	again, err := repo.ResolveOrCreateExternalIdentity(ctx, ExternalIdentityInput{
		TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: providerA, ProviderPluginID: "com.powerx.plugins.channel-a",
		ProviderSubject: "channel-user-1", DisplayName: "客户甲",
	})
	require.NoError(t, err)
	require.Equal(t, first.CustomerUUID, again.CustomerUUID)

	otherProvider, err := repo.ResolveOrCreateExternalIdentity(ctx, ExternalIdentityInput{
		TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: providerB, ProviderPluginID: "com.powerx.plugins.channel-b",
		ProviderSubject: "channel-user-1", DisplayName: "客户乙",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.CustomerUUID, otherProvider.CustomerUUID)

	var identities []modelcustomer.AuthIdentity
	require.NoError(t, db.Order("provider ASC").Find(&identities).Error)
	require.Len(t, identities, 2)
	require.Nil(t, identities[0].VerifiedAt)
	var metadata map[string]string
	require.NoError(t, json.Unmarshal(identities[0].Metadata, &metadata))
	require.NotEmpty(t, metadata["provider_plugin_id"])
	require.Equal(t, "plugin_attested", metadata["binding_kind"])

	var memberships []modelcustomer.TenantMembership
	require.NoError(t, db.Find(&memberships).Error)
	require.Len(t, memberships, 2)
	for _, membership := range memberships {
		require.Equal(t, "plugin_identity", membership.Source)
	}
}

func TestResolveOrCreateExternalIdentityRejectsTechnicalLabelsAndVerifiedBindings(t *testing.T) {
	db := newExternalIdentityRepositoryDB(t)
	repo := NewAccountRepository(db)
	ctx := context.Background()
	provider, err := ExternalIdentityProviderKey("com.powerx.plugins.channel-a")
	require.NoError(t, err)
	input := ExternalIdentityInput{
		TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: provider, ProviderPluginID: "com.powerx.plugins.channel-a",
		ProviderSubject: "channel-user-1", DisplayName: "客户甲",
	}
	created, err := repo.ResolveOrCreateExternalIdentity(ctx, input)
	require.NoError(t, err)

	input.ProviderSubject = "channel-user-2"
	input.DisplayName = ""
	_, err = repo.ResolveOrCreateExternalIdentity(ctx, input)
	require.ErrorIs(t, err, ErrExternalIdentityDisplayNameRequired)

	var identity modelcustomer.AuthIdentity
	require.NoError(t, db.Where("customer_uuid = ?", created.CustomerUUID).First(&identity).Error)
	verifiedAt := time.Now().UTC()
	require.NoError(t, db.Model(&modelcustomer.AuthIdentity{}).Where("uuid = ?", identity.UUID).Update("verified_at", verifiedAt).Error)
	input.ProviderSubject = "channel-user-1"
	input.DisplayName = "客户甲"
	_, err = repo.ResolveOrCreateExternalIdentity(ctx, input)
	require.ErrorIs(t, err, ErrExternalIdentityBindingUntrusted)
}

func TestResolveOrCreateExternalIdentityRejectsDisabledIdentityOrMembership(t *testing.T) {
	db := newExternalIdentityRepositoryDB(t)
	repo := NewAccountRepository(db)
	provider, err := ExternalIdentityProviderKey("com.powerx.plugins.channel-a")
	require.NoError(t, err)
	input := ExternalIdentityInput{
		TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: provider, ProviderPluginID: "com.powerx.plugins.channel-a",
		ProviderSubject: "channel-user-1", DisplayName: "客户甲",
	}
	created, err := repo.ResolveOrCreateExternalIdentity(context.Background(), input)
	require.NoError(t, err)

	require.NoError(t, db.Model(&modelcustomer.AuthIdentity{}).
		Where("customer_uuid = ?", created.CustomerUUID).
		Update("status", modelcustomer.StatusDisabled).Error)
	_, err = repo.ResolveOrCreateExternalIdentity(context.Background(), input)
	require.ErrorIs(t, err, ErrExternalIdentityBindingUntrusted)

	require.NoError(t, db.Model(&modelcustomer.AuthIdentity{}).
		Where("customer_uuid = ?", created.CustomerUUID).
		Update("status", modelcustomer.StatusActive).Error)
	require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).
		Where("uuid = ?", created.MembershipUUID).
		Update("status", modelcustomer.StatusDisabled).Error)
	_, err = repo.ResolveOrCreateExternalIdentity(context.Background(), input)
	require.ErrorIs(t, err, ErrExternalIdentityUnavailable)
}

func newExternalIdentityRepositoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, createExternalIdentityTestTables(db))
	return db
}

func createExternalIdentityTestTables(db *gorm.DB) error {
	for _, statement := range []string{
		`CREATE TABLE main.customer_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			status TEXT NOT NULL, primary_email TEXT, primary_phone TEXT, display_name TEXT, nickname TEXT, given_name TEXT,
			family_name TEXT, avatar_url TEXT, locale TEXT, timezone TEXT, metadata TEXT
		)`,
		`CREATE TABLE main.customer_auth_identities (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			customer_uuid TEXT NOT NULL, provider TEXT NOT NULL, provider_subject TEXT NOT NULL, email TEXT, phone TEXT,
			password_hash TEXT, status TEXT NOT NULL, verified_at DATETIME, metadata TEXT,
			UNIQUE(provider, provider_subject)
		)`,
		`CREATE TABLE main.customer_tenant_memberships (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, status TEXT NOT NULL, roles TEXT, scopes TEXT,
			source TEXT NOT NULL, expires_at DATETIME, metadata TEXT, UNIQUE(tenant_uuid, customer_uuid)
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
