package customer

import (
	"context"
	"errors"
	"testing"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	modelmetadata "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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

func TestCustomerCreateAtomicallyCreatesPrimaryContact(t *testing.T) {
	db, tenantUUID, _ := newContactServiceTestDB(t)
	service := NewAccountService(db)
	_, err := service.Create(context.Background(), CreateAccountInput{TenantUUID: tenantUUID, Type: modelcustomer.AccountTypeCompany, DisplayName: "Acme"})
	require.ErrorContains(t, err, "primary_contact_required")
	var before int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Count(&before).Error)
	_, err = service.Create(context.Background(), CreateAccountInput{TenantUUID: tenantUUID, Type: modelcustomer.AccountTypePerson, DisplayName: "Invalid", PrimaryEmail: "bad-address"})
	require.ErrorIs(t, err, ErrContactInvalidArgument)
	var after int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Count(&after).Error)
	require.Equal(t, before, after, "failed Contact must roll back Customer")

	person, err := service.Create(context.Background(), CreateAccountInput{TenantUUID: tenantUUID, Type: modelcustomer.AccountTypePerson, DisplayName: "Ada", PrimaryEmail: "ada@example.test"})
	require.NoError(t, err)
	require.Equal(t, modelcustomer.AccountTypePerson, person.Type)
	require.NotEmpty(t, person.PrimaryContactUUID)
	contact, err := NewContactService(db).Get(context.Background(), tenantUUID, person.UUID, person.PrimaryContactUUID)
	require.NoError(t, err)
	require.Equal(t, "Ada", contact.DisplayName)
	require.Equal(t, "ada@example.test", contact.Email)
	require.Contains(t, string(contact.Roles), `"primary"`)

	company, err := service.Create(context.Background(), CreateAccountInput{TenantUUID: tenantUUID, Type: modelcustomer.AccountTypeCompany, DisplayName: "Acme", PrimaryContact: &PrimaryContactInput{DisplayName: "Jane", Email: "jane@example.test"}})
	require.NoError(t, err)
	companyContact, err := NewContactService(db).Get(context.Background(), tenantUUID, company.UUID, company.PrimaryContactUUID)
	require.NoError(t, err)
	require.Equal(t, "Jane", companyContact.DisplayName)
	require.NotEqual(t, "Acme", companyContact.DisplayName)
}

func TestContactServiceWritesTransactionalAuditForContactChanges(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	service := NewContactService(db)
	ctx := reqctx.WithTraceID(context.Background(), "contact-audit-trace")
	ctx = context.WithValue(ctx, reqctx.KeyUserID, uint64(42))
	ctx = context.WithValue(ctx, "request_id", "contact-audit-request")
	contact, err := service.Create(ctx, CreateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, DisplayName: "Audited",
		CreationIntent: ContactCreationIntentExplicitCreate,
	})
	require.NoError(t, err)
	inactive := modelcustomer.ContactStatusInactive
	_, err = service.Update(ctx, UpdateContactInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: contact.UUID.String(), Status: &inactive,
	})
	require.NoError(t, err)

	var events []modelaudit.AuditEvent
	require.NoError(t, db.Where("tenant_uuid = ? AND resource_id = ?", tenantUUID, contact.UUID.String()).Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, []string{"customer.contact.created", "customer.contact.status_changed"}, []string{events[0].Operation, events[1].Operation})
	require.Equal(t, "contact-audit-trace", events[0].CorrelationID)
	require.NotNil(t, events[0].ActorUserID)
	require.EqualValues(t, 42, *events[0].ActorUserID)
	require.Contains(t, string(events[0].ChangesAfter), customerUUID)
	require.Contains(t, string(events[0].ChangesAfter), "contact-audit-request")
}

func TestContactServiceIdentityResolutionNeverCreatesAndRequiresExplicitTemporary(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	channelUUID := seedContactChannel(t, db, tenantUUID, "wecom")
	service := NewContactService(db)

	_, err := service.ResolveIdentity(context.Background(), ResolveContactIdentityInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, ChannelDictionaryItemUUID: channelUUID, ExternalSubject: "external-person-1",
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
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: contact.UUID.String(), ChannelDictionaryItemUUID: channelUUID, ExternalSubject: "external-person-1",
	})
	require.NoError(t, err)

	_, err = service.ResolveIdentity(context.Background(), ResolveContactIdentityInput{
		TenantUUID: tenantUUID, CustomerUUID: customerUUID, ChannelDictionaryItemUUID: channelUUID, ExternalSubject: "external-person-1",
	})
	// A temporary Contact is deliberately not resolved into a usable contact.
	require.ErrorIs(t, err, ErrContactIdentityNotFound)
}

func TestContactServiceRejectsUngovernedRolesTagsAndDuplicateIdentity(t *testing.T) {
	db, tenantUUID, customerUUID := newContactServiceTestDB(t)
	channelUUID := seedContactChannel(t, db, tenantUUID, "shopify")
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
	_, err = service.BindIdentity(context.Background(), BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: first.UUID.String(), ChannelDictionaryItemUUID: uuid.NewString(), ExternalSubject: "unapproved-channel"})
	require.ErrorIs(t, err, ErrContactChannelDictionaryInvalid)
	_, err = service.BindIdentity(context.Background(), BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: first.UUID.String(), ChannelDictionaryItemUUID: channelUUID, ExternalSubject: "shop:example:customer:1"})
	require.NoError(t, err)
	_, err = service.BindIdentity(context.Background(), BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: customerUUID, ContactUUID: second.UUID.String(), ChannelDictionaryItemUUID: channelUUID, ExternalSubject: "shop:example:customer:1"})
	require.ErrorIs(t, err, ErrContactIdentityConflict)
}

func newContactServiceTestDB(t *testing.T) (*gorm.DB, string, string) {
	t.Helper()
	db := newCustomerServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&modelaudit.AuditEvent{}))
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS main.customer_contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, display_name TEXT NOT NULL, given_name TEXT, family_name TEXT, email TEXT, phone TEXT,
			status TEXT NOT NULL, roles TEXT NOT NULL, tags TEXT NOT NULL, metadata TEXT NOT NULL
		)`,
		`CREATE TABLE main.customer_contact_identities (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, contact_uuid TEXT NOT NULL, channel_dictionary_item_uuid TEXT,
			external_subject TEXT NOT NULL, status TEXT NOT NULL, verified_at DATETIME, metadata TEXT NOT NULL,
			UNIQUE(tenant_uuid, channel_dictionary_item_uuid, external_subject)
		)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	require.NoError(t, db.AutoMigrate(&modelmetadata.DictionaryNamespace{}, &modelmetadata.DictionaryItem{}))
	tenantUUID, customerUUID := uuid.NewString(), uuid.NewString()
	seedActiveCustomer(t, db, tenantUUID, customerUUID)
	return db, tenantUUID, customerUUID
}

func seedContactChannel(t *testing.T, db *gorm.DB, tenantUUID, code string) string {
	t.Helper()
	namespaceUUID := uuid.NewString()
	require.NoError(t, db.Create(&modelmetadata.DictionaryNamespace{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(namespaceUUID)}, TenantUUID: tenantUUID,
		Namespace: ContactIdentityChannelNamespace, Module: "corex.customer", Status: "enabled",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"联系人渠道"}`)), DescriptionI18n: datatypes.JSON([]byte(`{}`)),
	}).Error)
	itemUUID := uuid.NewString()
	require.NoError(t, db.Create(&modelmetadata.DictionaryItem{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(itemUUID)}, TenantUUID: tenantUUID, NamespaceUUID: namespaceUUID,
		Code: code, LabelI18n: datatypes.JSON([]byte(`{"zh-CN":"渠道"}`)), DescriptionI18n: datatypes.JSON([]byte(`{}`)),
		Status: "enabled", Metadata: datatypes.JSON([]byte(`{}`)),
	}).Error)
	return itemUUID
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

func TestCustomerAccountSelectorUsesOnlyTypedCoreInternalContract(t *testing.T) {
	db, tenantUUID, _ := newContactServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db), NewContactService(db))
	request, err := decodeCustomerAccountListRequest(map[string]interface{}{"operation": "list", "page": 1, "page_size": 20})
	require.NoError(t, err)
	require.Equal(t, 20, request.PageSize)
	_, err = invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{
		CapabilityID: CustomerAccountsServiceReadCapabilityID, TenantUUID: tenantUUID, Method: "GET", Endpoint: "/api/v1/admin/customers/accounts",
	})
	require.ErrorIs(t, err, capabilityregistry.ErrCoreCapabilityNotHandled)
}

func TestCustomerAccountCreateCapabilityIsTypedAndTenantScoped(t *testing.T) {
	db, tenantUUID, _ := newContactServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db))
	body := map[string]interface{}{"operation": "create", "type": "person", "display_name": "Debug Customer", "primary_email": "debug-customer@example.test", "status": "active"}
	_, err := invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{CapabilityID: CustomerAccountsServiceReadCapabilityID, TenantUUID: tenantUUID, Method: "INVOKE", Endpoint: customerAccountsCoreEndpoint, Body: body})
	require.Error(t, err)
	_, err = invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{CapabilityID: CustomerAccountsServiceManageCapabilityID, TenantUUID: tenantUUID, Method: "POST", Endpoint: "/api/v1/admin/customers/accounts", Body: body})
	require.ErrorIs(t, err, capabilityregistry.ErrCoreCapabilityNotHandled)
	_, err = decodeCustomerAccountCreateRequest(map[string]interface{}{"operation": "create", "display_name": "Debug Customer", "tenant_uuid": tenantUUID})
	require.Error(t, err)
	result, err := invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{CapabilityID: CustomerAccountsServiceManageCapabilityID, TenantUUID: tenantUUID, Method: "INVOKE", Endpoint: customerAccountsCoreEndpoint, Body: body})
	require.NoError(t, err)
	require.NotNil(t, result["item"])
	var accountCount, membershipCount int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Where("primary_email = ?", "debug-customer@example.test").Count(&accountCount).Error)
	require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("tenant_uuid = ?", tenantUUID).Count(&membershipCount).Error)
	require.EqualValues(t, 1, accountCount)
	require.GreaterOrEqual(t, membershipCount, int64(2))
}
