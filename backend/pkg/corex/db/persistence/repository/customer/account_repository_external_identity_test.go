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
	require.Equal(t, first.PrimaryContactUUID, again.PrimaryContactUUID)
	require.Equal(t, modelcustomer.AccountTypePerson, again.Type)
	var contactCount int64
	require.NoError(t, db.Model(&modelcustomer.Contact{}).Where("tenant_uuid = ? AND customer_uuid = ?", "11111111-1111-1111-1111-111111111111", first.CustomerUUID).Count(&contactCount).Error)
	require.EqualValues(t, 1, contactCount)

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

func TestResolveOrCreateExternalIdentityRepairsMissingPrimaryContactIdempotently(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reuse_existing_%t", reuse), func(t *testing.T) {
			db := newExternalIdentityRepositoryDB(t)
			repo := NewAccountRepository(db)
			provider, err := ExternalIdentityProviderKey("com.powerx.plugins.channel-a")
			require.NoError(t, err)
			input := ExternalIdentityInput{TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: provider, ProviderPluginID: "com.powerx.plugins.channel-a", ProviderSubject: "channel-user-1", DisplayName: "客户甲", Email: "person@example.test"}
			first, err := repo.ResolveOrCreateExternalIdentity(context.Background(), input)
			require.NoError(t, err)
			require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", first.MembershipUUID).Update("primary_contact_uuid", "").Error)
			if !reuse {
				require.NoError(t, db.Where("uuid = ?", first.PrimaryContactUUID).Delete(&modelcustomer.Contact{}).Error)
			}
			second, err := repo.ResolveOrCreateExternalIdentity(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, first.CustomerUUID, second.CustomerUUID)
			if reuse {
				require.Equal(t, first.PrimaryContactUUID, second.PrimaryContactUUID)
			} else {
				require.NotEqual(t, first.PrimaryContactUUID, second.PrimaryContactUUID)
			}
			third, err := repo.ResolveOrCreateExternalIdentity(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, second.PrimaryContactUUID, third.PrimaryContactUUID)
			var count int64
			require.NoError(t, db.Model(&modelcustomer.Contact{}).Where("tenant_uuid = ? AND customer_uuid = ?", input.TenantUUID, first.CustomerUUID).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}

func TestResolveOrCreateExternalIdentityRefusesAmbiguousOrStaleContact(t *testing.T) {
	db := newExternalIdentityRepositoryDB(t)
	repo := NewAccountRepository(db)
	provider, err := ExternalIdentityProviderKey("com.powerx.plugins.channel-a")
	require.NoError(t, err)
	input := ExternalIdentityInput{TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: provider, ProviderPluginID: "com.powerx.plugins.channel-a", ProviderSubject: "channel-user-1", DisplayName: "客户甲"}
	first, err := repo.ResolveOrCreateExternalIdentity(context.Background(), input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", first.MembershipUUID).Update("primary_contact_uuid", "").Error)
	extra := modelcustomer.Contact{TenantUUID: input.TenantUUID, CustomerUUID: first.CustomerUUID, DisplayName: "另一人", Status: modelcustomer.ContactStatusActive, Roles: []byte(`[]`), Tags: []byte(`[]`), Metadata: []byte(`{}`)}
	require.NoError(t, db.Create(&extra).Error)
	require.NoError(t, db.Model(&modelcustomer.Account{}).Where("uuid = ?", first.CustomerUUID).Update("type", "").Error)
	_, err = repo.ResolveOrCreateExternalIdentity(context.Background(), input)
	require.ErrorIs(t, err, ErrExternalIdentityContactAmbiguous)
	var unchanged modelcustomer.Account
	require.NoError(t, db.Where("uuid = ?", first.CustomerUUID).First(&unchanged).Error)
	require.Empty(t, unchanged.Type)
	require.NoError(t, db.Delete(&extra).Error)
	require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", first.MembershipUUID).Update("primary_contact_uuid", extra.UUID.String()).Error)
	_, err = repo.ResolveOrCreateExternalIdentity(context.Background(), input)
	require.ErrorIs(t, err, ErrExternalIdentityContactRequired)
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
			type TEXT, status TEXT NOT NULL, primary_email TEXT, primary_phone TEXT, display_name TEXT, nickname TEXT, given_name TEXT,
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
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, primary_contact_uuid TEXT, status TEXT NOT NULL, roles TEXT, scopes TEXT,
			source TEXT NOT NULL, expires_at DATETIME, metadata TEXT, UNIQUE(tenant_uuid, customer_uuid)
		)`,
		`CREATE TABLE main.customer_contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, display_name TEXT NOT NULL, given_name TEXT, family_name TEXT,
			email TEXT, phone TEXT, status TEXT NOT NULL, roles TEXT NOT NULL, tags TEXT NOT NULL, metadata TEXT NOT NULL
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func TestVerifiedIdentityDefaultsMissingTypeAndPreservesCompany(t *testing.T) {
	for _, kind := range []string{"", "company"} {
		t.Run("type_"+kind, func(t *testing.T) {
			db := newExternalIdentityRepositoryDB(t)
			repo := NewAccountRepository(db)
			provider, err := ExternalIdentityProviderKey("com.powerx.test.default")
			require.NoError(t, err)
			in := ExternalIdentityInput{TenantUUID: "11111111-1111-1111-1111-111111111111", ProviderKey: provider, ProviderPluginID: "com.powerx.test.default", ProviderSubject: "subject", DisplayName: "Saved name", Email: "saved@example.test", Phone: "123456"}
			first, err := repo.ResolveOrCreateExternalIdentity(context.Background(), in)
			require.NoError(t, err)
			require.NoError(t, db.Model(&modelcustomer.Account{}).Where("uuid = ?", first.CustomerUUID).Update("type", kind).Error)
			require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", first.MembershipUUID).Update("primary_contact_uuid", nil).Error)
			if kind == "" {
				require.NoError(t, db.Unscoped().Where("uuid = ?", first.PrimaryContactUUID).Delete(&modelcustomer.Contact{}).Error)
			}
			in.DisplayName = "Incoming name"
			in.Email = "incoming@example.test"
			in.Phone = "999"
			second, err := repo.ResolveOrCreateExternalIdentity(context.Background(), in)
			require.NoError(t, err)
			third, err := repo.ResolveOrCreateExternalIdentity(context.Background(), in)
			require.NoError(t, err)
			require.Equal(t, first.CustomerUUID, second.CustomerUUID)
			require.Equal(t, second, third)
			expected := kind
			if expected == "" {
				expected = "person"
			}
			require.Equal(t, expected, second.Type)
			var stored modelcustomer.Account
			require.NoError(t, db.Where("uuid = ?", first.CustomerUUID).First(&stored).Error)
			require.Equal(t, expected, stored.Type)
			var contact modelcustomer.Contact
			require.NoError(t, db.Where("uuid = ?", second.PrimaryContactUUID).First(&contact).Error)
			require.Equal(t, "saved@example.test", contact.Email)
			require.Equal(t, "123456", contact.Phone)
			require.Equal(t, "Saved name", contact.DisplayName)
			if kind == "company" {
				require.Equal(t, first.PrimaryContactUUID, second.PrimaryContactUUID)
			}
			var count int64
			require.NoError(t, db.Model(&modelcustomer.Contact{}).Where("customer_uuid = ?", first.CustomerUUID).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
