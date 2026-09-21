package customer

import (
	"context"
	"errors"
	"testing"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestContactServiceCreatesAndScopesContact(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	otherCustomerUUID := uuid.NewString()
	seedActiveCustomer(t, db, tenantUUID, otherCustomerUUID)
	service := NewContactService(db)

	contact, err := service.Create(context.Background(), CreateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Chen Li",
		Roles: []string{modelcustomer.ContactRolePrimary}, Tags: []string{"vip"},
		CreationIntent: ContactCreationIntentExplicitCreate,
	})
	require.NoError(t, err)
	require.NotEmpty(t, contact.UUID)
	require.Equal(t, modelcustomer.ContactStatusActive, contact.Status)

	got, err := service.Get(context.Background(), tenantUUID, customerUUID, contact.UUID.String())
	require.NoError(t, err)
	require.Equal(t, contact.UUID, got.UUID)

	_, err = service.Get(context.Background(), tenantUUID, otherCustomerUUID, contact.UUID.String())
	require.ErrorIs(t, err, ErrContactCustomerMismatch)

	page, err := service.ListByCustomer(context.Background(), ListContactsInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
}

func TestContactServiceIdentityResolutionNeverCreatesAndRequiresExplicitTemporary(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	service := NewContactService(db)

	_, err := service.ResolveIdentity(context.Background(), ResolveContactIdentityInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, Channel: "wecom", ExternalSubject: "external-person-1",
	})
	require.ErrorIs(t, err, ErrContactIdentityNotFound)
	var contactCount int64
	require.NoError(t, db.Model(&modelcustomer.Contact{}).Count(&contactCount).Error)
	require.Zero(t, contactCount)

	_, err = service.Create(context.Background(), CreateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Temp", Status: modelcustomer.ContactStatusTemporary,
		CreationIntent: ContactCreationIntentExplicitCreate,
	})
	require.ErrorIs(t, err, ErrContactInvalidArgument)

	contact, err := service.Create(context.Background(), CreateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Temp", Status: modelcustomer.ContactStatusTemporary,
		CreationIntent: ContactCreationIntentExplicitTemporary,
	})
	require.NoError(t, err)
	_, err = service.BindIdentity(context.Background(), BindContactIdentityInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: contact.UUID.String(), Channel: "wecom", ExternalSubject: "external-person-1",
	})
	require.NoError(t, err)

	_, err = service.ResolveIdentity(context.Background(), ResolveContactIdentityInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, Channel: "wecom", ExternalSubject: "external-person-1",
	})
	// A temporary Contact is deliberately not resolved into a usable contact.
	require.ErrorIs(t, err, ErrContactIdentityNotFound)
}

func TestContactServiceRejectsUngovernedRolesTagsAndDuplicateIdentity(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	service := NewContactService(db)

	_, err := service.Create(context.Background(), CreateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Role", Roles: []string{"designer"}, CreationIntent: ContactCreationIntentExplicitCreate,
	})
	require.ErrorIs(t, err, ErrContactInvalidArgument)
	_, err = service.Create(context.Background(), CreateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Tag", Tags: []string{"Not Valid"}, CreationIntent: ContactCreationIntentExplicitCreate,
	})
	require.ErrorIs(t, err, ErrContactInvalidArgument)

	first, err := service.Create(context.Background(), CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "First", CreationIntent: ContactCreationIntentExplicitCreate})
	require.NoError(t, err)
	second, err := service.Create(context.Background(), CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Second", CreationIntent: ContactCreationIntentExplicitCreate})
	require.NoError(t, err)
	_, err = service.BindIdentity(context.Background(), BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: first.UUID.String(), Channel: "shopify", ExternalSubject: "shop:example:customer:1"})
	require.NoError(t, err)
	_, err = service.BindIdentity(context.Background(), BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: second.UUID.String(), Channel: "shopify", ExternalSubject: "shop:example:customer:1"})
	require.ErrorIs(t, err, ErrContactIdentityConflict)
}

func newContactServiceTestDB(t *testing.T) (*gorm.DB, string, string) {
	t.Helper()
	db := newCustomerServiceTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE main.customer_contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, display_name TEXT NOT NULL, given_name TEXT, family_name TEXT,
			status TEXT NOT NULL, roles TEXT NOT NULL, tags TEXT NOT NULL, metadata TEXT NOT NULL
		)`,
		`CREATE TABLE main.customer_contact_identities (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, contact_uuid TEXT NOT NULL, channel TEXT NOT NULL,
			external_subject TEXT NOT NULL, status TEXT NOT NULL, verified_at DATETIME, metadata TEXT NOT NULL,
			UNIQUE(tenant_uuid, channel, external_subject)
		)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	tenantUUID, customerUUID := uuid.NewString(), uuid.NewString()
	seedActiveCustomer(t, db, tenantUUID, customerUUID)
	return db, tenantUUID, customerUUID
}

func seedActiveCustomer(t *testing.T, db *gorm.DB, tenantUUID, customerUUID string) {
	t.Helper()
	parsedCustomerUUID, err := uuid.Parse(customerUUID)
	require.NoError(t, err)
	require.NoError(t, db.Create(&modelcustomer.Account{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: parsedCustomerUUID}, Status: modelcustomer.StatusActive, DisplayName: "Customer",
		Metadata: datatypes.JSON([]byte("{}")),
	}).Error)
	require.NoError(t, db.Create(&modelcustomer.TenantMembership{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, Status: modelcustomer.StatusActive, Source: "test",
		Roles: datatypes.JSON([]byte("[]")), Scopes: datatypes.JSON([]byte("[]")), Metadata: datatypes.JSON([]byte("{}")),
	}).Error)
}

func TestContactServiceRejectsInactiveCustomerMembership(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).
		Where("tenant_uuid = ? AND customer_uuid = ?", tenantUUID, customerUUID).
		Update("status", modelcustomer.StatusSuspended).Error)
	_, err := NewContactService(db).Create(context.Background(), CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Nope", CreationIntent: ContactCreationIntentExplicitCreate})
	require.True(t, errors.Is(err, ErrContactCustomerMembershipInactive))
}

func TestContactCapabilityInvokerLocksCapabilityToTypedOperation(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db), NewContactService(db))
	displayName := "Contact"
	result, err := invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{
		CapabilityID: CustomerContactsServiceManageCapabilityID, TenantUUID: tenantUUID, Method: "INVOKE", Endpoint: CustomerContactsCoreEndpoint,
		Body: map[string]interface{}{"operation": "create", "customer_uuid": customerUUID, "display_name": displayName, "creation_intent": ContactCreationIntentExplicitCreate},
	})
	require.NoError(t, err)
	require.NotNil(t, result["item"])
	_, err = invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{
		CapabilityID: CustomerContactsServiceReadCapabilityID, TenantUUID: tenantUUID, Method: "INVOKE", Endpoint: CustomerContactsCoreEndpoint,
		Body: map[string]interface{}{"operation": "create", "customer_uuid": customerUUID, "display_name": displayName, "creation_intent": ContactCreationIntentExplicitCreate},
	})
	require.ErrorIs(t, err, capabilityregistry.ErrCoreCapabilityNotHandled)
}
