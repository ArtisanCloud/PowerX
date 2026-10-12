package customer

import (
	"context"
	"strings"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPrimaryContactReferenceAndRoleAreAtomic(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	ctx := importAdminContext(tenant)
	a, err := NewAccountService(db).Create(ctx, CreateAccountInput{TenantUUID: tenant, Type: "person", DisplayName: "First"})
	require.NoError(t, err)
	s := NewContactService(db)
	second, err := s.Create(ctx, CreateContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, DisplayName: "Second", CreationIntent: ContactCreationIntentExplicitCreate})
	require.NoError(t, err)
	inactive := "inactive"
	_, err = s.Update(ctx, UpdateContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, ContactUUID: a.PrimaryContactUUID, Status: &inactive})
	require.Equal(t, "CONTACT_PRIMARY_CHANGE_REQUIRED", dto.CodeOf(err))
	roles := []string{"primary"}
	_, err = s.Update(ctx, UpdateContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, ContactUUID: second.UUID.String(), Roles: &roles})
	require.Equal(t, "CONTACT_PRIMARY_OPERATION_REQUIRED", dto.CodeOf(err))
	empty := []string{}
	_, err = s.Update(ctx, UpdateContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, ContactUUID: a.PrimaryContactUUID, Roles: &empty})
	require.Equal(t, "CONTACT_PRIMARY_OPERATION_REQUIRED", dto.CodeOf(err))
	input := SetPrimaryContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, ContactUUID: second.UUID.String(), ExpectedPrimaryContactUUID: a.PrimaryContactUUID}
	input.ExpectedPrimaryContactUUID = uuid.NewString()
	_, err = s.SetPrimaryContact(ctx, input)
	require.Equal(t, "CONTACT_PRIMARY_CHANGED", dto.CodeOf(err))
	input.ExpectedPrimaryContactUUID = a.PrimaryContactUUID
	result, err := s.SetPrimaryContact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, a.PrimaryContactUUID, result.PreviousPrimaryContactUUID)
	var read model.TenantMembership
	require.NoError(t, db.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, a.UUID).First(&read).Error)
	require.Equal(t, second.UUID.String(), read.PrimaryContactUUID)
	old, err := s.Get(ctx, tenant, a.UUID, a.PrimaryContactUUID)
	require.NoError(t, err)
	require.NotContains(t, string(old.Roles), `"primary"`)
	newPrimary, err := s.Get(ctx, tenant, a.UUID, second.UUID.String())
	require.NoError(t, err)
	require.Contains(t, string(newPrimary.Roles), `"primary"`)
	_, err = s.Update(ctx, UpdateContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, ContactUUID: old.UUID.String(), Status: &inactive})
	require.NoError(t, err)
	_, err = s.SetPrimaryContact(ctx, SetPrimaryContactInput{TenantUUID: tenant, CustomerUUID: a.UUID, ContactUUID: old.UUID.String(), ExpectedPrimaryContactUUID: read.PrimaryContactUUID})
	require.Equal(t, "CONTACT_PRIMARY_NOT_ACTIVE", dto.CodeOf(err))
}

func TestPrimaryAuditFailureRollsBackAndTypedCapabilityIsSeparate(t *testing.T) {
	db, tenant, customer := newContactServiceTestDB(t)
	s := NewContactService(db)
	c, err := s.Create(context.Background(), CreateContactInput{TenantUUID: tenant, CustomerUUID: customer, DisplayName: "Target", CreationIntent: ContactCreationIntentExplicitCreate})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TRIGGER primary_audit_failure BEFORE INSERT ON audit_event WHEN NEW.operation='customer.contact.primary_changed' BEGIN SELECT RAISE(ABORT,'injected'); END").Error)
	_, err = s.SetPrimaryContact(context.Background(), SetPrimaryContactInput{TenantUUID: tenant, CustomerUUID: customer, ContactUUID: c.UUID.String()})
	require.Error(t, err)
	require.Equal(t, "CONTACT_PRIMARY_OPERATION_FAILED", dto.CodeOf(err))
	var membership model.TenantMembership
	require.NoError(t, db.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, customer).First(&membership).Error)
	require.Empty(t, membership.PrimaryContactUUID)
	read, err := s.Get(context.Background(), tenant, customer, c.UUID.String())
	require.NoError(t, err)
	require.NotContains(t, string(read.Roles), `"primary"`)
	require.NoError(t, db.Exec("DROP TRIGGER primary_audit_failure").Error)
	invoker := NewCapabilityInvoker(NewAccountService(db), s)
	in := cap.CoreCapabilityInvokeInput{CapabilityID: CustomerContactsServiceManageCapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: CustomerContactsCoreEndpoint, Body: map[string]any{"operation": "set_primary", "customer_uuid": customer, "contact_uuid": c.UUID.String(), "expected_primary_contact_uuid": ""}}
	_, err = invoker.InvokeCoreCapability(context.Background(), in)
	require.ErrorIs(t, err, cap.ErrCoreCapabilityNotHandled)
	in.CapabilityID = CustomerContactsPrimaryManageCapabilityID
	in.Body["headers"] = map[string]any{}
	_, err = invoker.InvokeCoreCapability(context.Background(), in)
	require.ErrorIs(t, err, ErrContactInvalidArgument)
	delete(in.Body, "headers")
	_, err = invoker.InvokeCoreCapability(context.Background(), in)
	require.NoError(t, err)
	delete(in.Body, "expected_primary_contact_uuid")
	_, err = invoker.InvokeCoreCapability(context.Background(), in)
	require.ErrorIs(t, err, ErrContactInvalidArgument)
}

func TestContactRepairPreviewCommitIsolationAndIdempotency(t *testing.T) {
	db, tenant, customer := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AccountImport{}))
	ctx := importAdminContext(tenant)
	s := NewContactRepairService(db)
	preview, err := s.Preview(ctx, ContactRepairInput{CustomerUUID: customer, Type: "person"})
	require.NoError(t, err)
	require.Equal(t, "create_contact", preview.Proposal.Action)
	var count int64
	require.NoError(t, db.Model(&model.Contact{}).Count(&count).Error)
	require.Zero(t, count)
	_, err = s.Get(importAdminContext(uuid.NewString()), preview.UUID)
	require.Equal(t, 404, dto.StatusCode(err))
	_, err = NewAccountImportService(db).Get(ctx, preview.UUID)
	require.Equal(t, 404, dto.StatusCode(err))
	_, err = s.Commit(ctx, preview.UUID, preview.Checksum, false)
	require.Equal(t, "CONTACT_REPAIR_CONFIRM_REQUIRED", dto.CodeOf(err))
	result, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.NotEmpty(t, result.PrimaryContactUUID)
	again, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, result, again)
	var read model.TenantMembership
	require.NoError(t, db.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, customer).First(&read).Error)
	require.Equal(t, result.PrimaryContactUUID, read.PrimaryContactUUID)
	var account model.Account
	require.NoError(t, db.Where("uuid = ?", customer).First(&account).Error)
	require.Equal(t, "person", account.Type)
	require.NoError(t, db.Model(&model.Contact{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.Account{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestContactRepairConflictsRequirementsAndRollback(t *testing.T) {
	db, tenant, customer := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AccountImport{}))
	ctx := importAdminContext(tenant)
	s := NewContactRepairService(db)
	_, err := s.Preview(ctx, ContactRepairInput{CustomerUUID: customer, Type: "company"})
	require.Equal(t, "CONTACT_REPAIR_PERSON_REQUIRED", dto.CodeOf(err))
	input := ContactRepairInput{CustomerUUID: customer, Type: "person"}
	p, err := s.Preview(ctx, input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Account{}).Where("uuid = ?", customer).Update("display_name", "changed").Error)
	_, err = s.Commit(ctx, p.UUID, p.Checksum, true)
	require.Equal(t, "CONTACT_REPAIR_SOURCE_CHANGED", dto.CodeOf(err))
	p, err = s.Preview(ctx, input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AccountImport{}).Where("uuid = ?", p.UUID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	_, err = s.Commit(ctx, p.UUID, p.Checksum, true)
	require.Equal(t, "CONTACT_REPAIR_EXPIRED", dto.CodeOf(err))
	p, err = s.Preview(ctx, input)
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TRIGGER repair_audit_failure BEFORE INSERT ON audit_event WHEN NEW.operation='customer.contact_repair.committed' BEGIN SELECT RAISE(ABORT,'injected'); END").Error)
	_, err = s.Commit(ctx, p.UUID, p.Checksum, true)
	require.Equal(t, "CONTACT_REPAIR_COMMIT_FAILED", dto.CodeOf(err))
	var count int64
	require.NoError(t, db.Model(&model.Contact{}).Count(&count).Error)
	require.Zero(t, count)
	read, err := s.Get(ctx, p.UUID)
	require.NoError(t, err)
	require.Equal(t, "preview", read.Status)
	require.NoError(t, db.Exec("DROP TRIGGER repair_audit_failure").Error)
	_, err = s.Commit(ctx, p.UUID, p.Checksum, true)
	require.NoError(t, err)
}

func TestCustomerUnicodeValidationMatchesPreviewAndCommit(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AccountImport{}))
	ctx := importAdminContext(tenant)
	s := NewAccountImportService(db)
	for _, length := range []int{43, 128, 129} {
		name := strings.Repeat("中", length)
		preview, err := s.Preview(ctx, "unicode.csv", "display_name,primary_email\n"+name+",n"+strings.Repeat("x", length)+"@example.test\n")
		require.NoError(t, err)
		if length == 129 {
			require.Equal(t, "invalid", preview.Rows[0].Status)
			continue
		}
		require.Equal(t, "ready", preview.Rows[0].Status)
		result, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
		require.NoError(t, err)
		require.Equal(t, 1, result.Created)
		contact, err := NewContactService(db).Get(ctx, tenant, result.Rows[0].CustomerUUID, result.Rows[0].PrimaryContactUUID)
		require.NoError(t, err)
		require.Equal(t, name, contact.DisplayName)
	}
	require.False(t, validContactNames("A\x00B", "", ""))
	require.False(t, validContactNames(string([]byte{255}), "", ""))
}

func TestContactRepairReusesExplicitContactAndRejectsAmbiguity(t *testing.T) {
	db, tenant, customer := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AccountImport{}))
	ctx := importAdminContext(tenant)
	for _, name := range []string{"First", "Second"} {
		_, err := NewContactService(db).Create(ctx, CreateContactInput{TenantUUID: tenant, CustomerUUID: customer, DisplayName: name, CreationIntent: ContactCreationIntentExplicitCreate})
		require.NoError(t, err)
	}
	s := NewContactRepairService(db)
	_, err := s.Preview(ctx, ContactRepairInput{CustomerUUID: customer, Type: "company"})
	require.Equal(t, "CONTACT_REPAIR_AMBIGUOUS", dto.CodeOf(err))
	var chosen model.Contact
	require.NoError(t, db.Where("display_name = ?", "Second").First(&chosen).Error)
	_, err = s.Preview(ctx, ContactRepairInput{CustomerUUID: customer, Type: "company", ContactUUID: uuid.NewString()})
	require.Equal(t, "CONTACT_REPAIR_CONTACT_INVALID", dto.CodeOf(err))
	p, err := s.Preview(ctx, ContactRepairInput{CustomerUUID: customer, Type: "company", ContactUUID: chosen.UUID.String()})
	require.NoError(t, err)
	require.Equal(t, "reuse_contact", p.Proposal.Action)
	result, err := s.Commit(ctx, p.UUID, p.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, chosen.UUID.String(), result.PrimaryContactUUID)
	var count int64
	require.NoError(t, db.Model(&model.Contact{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	p, err = s.Preview(ctx, ContactRepairInput{CustomerUUID: customer, Type: "person"})
	require.Equal(t, "CONTACT_REPAIR_TYPE_CHANGED", dto.CodeOf(err))
}
