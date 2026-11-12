package customer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type verificationFixture struct {
	db                                    *gorm.DB
	ctx                                   context.Context
	tenant, customer, membership, contact string
	now                                   time.Time
	service                               *MembershipVerificationService
}

func newVerificationFixture(t *testing.T) *verificationFixture {
	t.Helper()
	f := &verificationFixture{db: newCustomerServiceTestDB(t), tenant: uuid.NewString(), customer: uuid.NewString(), membership: uuid.NewString(), contact: uuid.NewString(), now: time.Now().UTC().Truncate(time.Second)}
	require.NoError(t, f.db.AutoMigrate(&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &setting.PluginInstanceConfig{}))
	f.seed(t)
	return f
}

func (f *verificationFixture) seed(t *testing.T) {
	t.Helper()
	row := func(id string) coremodel.PowerUUIDModel {
		return coremodel.PowerUUIDModel{UUID: uuid.MustParse(id), CreatedAt: f.now, UpdatedAt: f.now}
	}
	require.NoError(t, f.db.Create(&m.Account{PowerUUIDModel: row(f.customer), Type: "person", Status: "active", DisplayName: "Private label", PrimaryEmail: "private@example.test"}).Error)
	require.NoError(t, f.db.Create(&m.TenantMembership{PowerUUIDModel: row(f.membership), TenantUUID: f.tenant, CustomerUUID: f.customer, PrimaryContactUUID: f.contact, Status: "active", Source: "fixture"}).Error)
	require.NoError(t, f.db.Create(&m.Contact{PowerUUIDModel: row(f.contact), TenantUUID: f.tenant, CustomerUUID: f.customer, Status: "active", DisplayName: "Private contact", Email: "private-contact@example.test", Roles: datatypes.JSON(`[]`), Tags: datatypes.JSON(`[]`), Metadata: datatypes.JSON(`{}`)}).Error)
	require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: CustomerMembershipsServiceReadCapabilityID, PluginID: "corex.platform", PluginVersion: "1.0.0", Title: "Verify", CapabilitiesHash: "fixture", ProtocolHash: "fixture", Status: "published"}).Error)
	require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: CustomerMembershipsServiceReadCapabilityID, TenantUUID: f.tenant, ContractRef: "fixture", Status: "published", Version: 1}).Error)
	require.NoError(t, f.db.Create(&setting.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: "com.example.checkout", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"verify-fixture","allowed_capabilities":["com.corex.customer.memberships.service_read"]}`)}).Error)
	f.ctx = reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &reqctx.CoreXClaims{TenantUUID: f.tenant, PluginID: "com.example.checkout", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:verify-fixture", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	f.service = NewMembershipVerificationService(f.db)
	f.service.now = func() time.Time { return f.now }
}

func TestMembershipVerificationSnapshotAndRevision(t *testing.T) {
	f := newVerificationFixture(t)
	writes := 0
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("verify_read_only", func(*gorm.DB) { writes++ }))
	require.NoError(t, f.db.Callback().Update().Before("gorm:update").Register("verify_read_only", func(*gorm.DB) { writes++ }))
	require.NoError(t, f.db.Callback().Delete().Before("gorm:delete").Register("verify_read_only", func(*gorm.DB) { writes++ }))
	first, err := f.service.VerifyCustomer(f.ctx, f.customer)
	require.NoError(t, err)
	require.Equal(t, 1, first.SchemaVersion)
	require.Equal(t, f.tenant, first.TenantUUID)
	require.Equal(t, f.customer, first.CustomerUUID)
	require.Equal(t, f.membership, first.MembershipUUID)
	require.Equal(t, f.contact, first.PrimaryContactUUID)
	require.Equal(t, "active", first.Status)
	require.Nil(t, first.ExpiresAt)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, first.Revision)
	f.service.now = func() time.Time { return f.now.Add(time.Second) }
	second, err := f.service.VerifyCustomer(f.ctx, f.customer)
	require.NoError(t, err)
	require.Equal(t, first.Revision, second.Revision)
	require.NotEqual(t, first.CheckedAt, second.CheckedAt)
	require.Zero(t, writes, "核验不能写入业务数据")
	raw, err := json.Marshal(first)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"expires_at":null`)
	require.NotContains(t, string(raw), "private")
	require.NoError(t, f.db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("updated_at", f.now.Add(time.Minute)).Error)
	third, err := f.service.VerifyCustomer(f.ctx, f.customer)
	require.NoError(t, err)
	require.NotEqual(t, first.Revision, third.Revision)
}

func TestMembershipVerificationRejectsInvalidState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *verificationFixture)
		want   error
	}{
		{"unknown", func(t *testing.T, f *verificationFixture) { f.customer = uuid.NewString() }, ErrVerificationNotFound},
		{"other_tenant", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("tenant_uuid", uuid.NewString()).Error)
		}, ErrVerificationNotFound},
		{"deleted_customer", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Where("uuid=?", f.customer).Delete(&m.Account{}).Error)
		}, ErrVerificationNotFound},
		{"deleted_membership", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Where("uuid=?", f.membership).Delete(&m.TenantMembership{}).Error)
		}, ErrVerificationNotFound},
		{"inactive_customer", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.Account{}).Where("uuid=?", f.customer).Update("status", "suspended").Error)
		}, ErrVerificationCustomerInactive},
		{"inactive_membership", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("status", "disabled").Error)
		}, ErrVerificationMembershipInactive},
		{"expired", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("expires_at", f.now.Add(-time.Second)).Error)
		}, ErrVerificationMembershipExpired},
		{"expiry_boundary", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("expires_at", f.now).Error)
		}, ErrVerificationMembershipExpired},
		{"missing_primary", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.TenantMembership{}).Where("uuid=?", f.membership).Update("primary_contact_uuid", nil).Error)
		}, ErrVerificationPrimaryContactInvalid},
		{"missing_contact", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Where("uuid=?", f.contact).Delete(&m.Contact{}).Error)
		}, ErrVerificationPrimaryContactInvalid},
		{"foreign_contact_tenant", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("tenant_uuid", uuid.NewString()).Error)
		}, ErrVerificationPrimaryContactInvalid},
		{"foreign_contact_customer", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("customer_uuid", uuid.NewString()).Error)
		}, ErrVerificationPrimaryContactInvalid},
		{"inactive_contact", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("status", "inactive").Error)
		}, ErrVerificationPrimaryContactInvalid},
		{"temporary_contact", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&m.Contact{}).Where("uuid=?", f.contact).Update("status", "temporary").Error)
		}, ErrVerificationPrimaryContactInvalid},
		{"data_dependency", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Exec("DROP TABLE main.customer_contacts").Error)
		}, ErrVerificationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newVerificationFixture(t)
			tc.mutate(t, f)
			result, err := f.service.VerifyCustomer(f.ctx, f.customer)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, result.CustomerUUID)
		})
	}
}

func TestMembershipVerificationLiveAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *verificationFixture)
		want   error
	}{
		{"no_claims", func(t *testing.T, f *verificationFixture) { f.ctx = context.Background() }, ErrVerificationUnauthorized},
		{"unverified_api_key_context", func(t *testing.T, f *verificationFixture) {
			f.ctx = reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &reqctx.CoreXClaims{TenantUUID: f.tenant, Platforms: []string{"api_key"}})
		}, ErrVerificationUnauthorized},
		{"admin", func(t *testing.T, f *verificationFixture) {
			f.ctx = reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &reqctx.CoreXClaims{TenantUUID: f.tenant, UserID: 1, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx"}})
		}, ErrVerificationUnauthorized},
		{"tenant_override", func(t *testing.T, f *verificationFixture) { f.ctx = reqctx.WithTenantUUID(f.ctx, uuid.NewString()) }, ErrVerificationUnauthorized},
		{"expired_service_token", func(t *testing.T, f *verificationFixture) {
			reqctx.GetClaims(f.ctx).ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
		}, ErrVerificationUnauthorized},
		{"wrong_service_subject", func(t *testing.T, f *verificationFixture) {
			reqctx.GetClaims(f.ctx).Subject = "client:another-service"
		}, ErrVerificationUnauthorized},
		{"unpublished", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", CustomerMembershipsServiceReadCapabilityID).Update("status", "draft").Error)
		}, ErrVerificationForbidden},
		{"unregistered", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Where("tenant_uuid=?", f.tenant).Delete(&cm.CapabilityRegistration{}).Error)
		}, ErrVerificationForbidden},
		{"latest_registration_disabled", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: CustomerMembershipsServiceReadCapabilityID, TenantUUID: f.tenant, ContractRef: "v2", Status: "disabled", Version: 2}).Error)
		}, ErrVerificationForbidden},
		{"revoked", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid=?", f.tenant).Update("enabled", false).Error)
		}, ErrVerificationForbidden},
		{"authorization_dependency", func(t *testing.T, f *verificationFixture) {
			require.NoError(t, f.db.Exec("DROP TABLE main.plugin_instance_configs").Error)
		}, ErrVerificationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newVerificationFixture(t)
			tc.mutate(t, f)
			_, err := f.service.VerifyCustomer(f.ctx, f.customer)
			require.ErrorIs(t, err, tc.want)
		})
	}
	f := newVerificationFixture(t)
	_, err := f.service.VerifyCustomer(f.ctx, f.customer)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid=?", f.tenant).Update("value_json", datatypes.JSON(`{"client_id":"verify-fixture","allowed_capabilities":[]}`)).Error)
	_, err = f.service.VerifyCustomer(f.ctx, f.customer)
	require.ErrorIs(t, err, ErrVerificationForbidden)
}

func TestMembershipVerificationTypedInvocation(t *testing.T) {
	f := newVerificationFixture(t)
	invoker := NewCapabilityInvoker(NewAccountService(f.db))
	valid := map[string]any{"operation": "verify_customer", "schema_version": 1, "customer_uuid": f.customer}
	invoke := func(body map[string]any) (map[string]any, error) {
		return invoker.InvokeCoreCapability(f.ctx, cap.CoreCapabilityInvokeInput{CapabilityID: CustomerMembershipsServiceReadCapabilityID, TenantUUID: f.tenant, Method: "INVOKE", Endpoint: CustomerMembershipsCoreEndpoint, Body: body})
	}
	out, err := invoke(valid)
	require.NoError(t, err)
	require.Equal(t, f.customer, out["item"].(CustomerVerification).CustomerUUID)
	for _, field := range []string{"tenant_uuid", "plugin_id", "actor", "headers", "method", "endpoint", "Customer_UUID"} {
		bad := map[string]any{}
		for k, v := range valid {
			bad[k] = v
		}
		bad[field] = "forbidden"
		_, err := invoke(bad)
		require.ErrorIs(t, err, ErrVerificationInvalidArgument)
	}
	for _, change := range []map[string]any{{"operation": "list"}, {"operation": nil}, {"schema_version": 2}, {"schema_version": "1"}, {"schema_version": 1.5}, {"customer_uuid": uuid.Nil.String()}, {"customer_uuid": "11111111-1111-4111-8111-ABCDEF123456"}, {"customer_uuid": " " + f.customer}} {
		bad := map[string]any{}
		for k, v := range valid {
			bad[k] = v
		}
		for k, v := range change {
			bad[k] = v
		}
		_, err := invoke(bad)
		require.ErrorIs(t, err, ErrVerificationInvalidArgument)
	}
	for _, mutate := range []func(*cap.CoreCapabilityInvokeInput){
		func(in *cap.CoreCapabilityInvokeInput) { in.Endpoint = "https://attacker.invalid" },
		func(in *cap.CoreCapabilityInvokeInput) { in.Endpoint += "/" },
		func(in *cap.CoreCapabilityInvokeInput) { in.Method = "GET" },
		func(in *cap.CoreCapabilityInvokeInput) {
			in.Payload = map[string]any{"endpoint": CustomerMembershipsCoreEndpoint}
		},
		func(in *cap.CoreCapabilityInvokeInput) { in.Context = map[string]any{"actor": "admin"} },
		func(in *cap.CoreCapabilityInvokeInput) { in.Query = map[string]string{"tenant_uuid": f.tenant} },
	} {
		in := cap.CoreCapabilityInvokeInput{CapabilityID: CustomerMembershipsServiceReadCapabilityID, TenantUUID: f.tenant, Method: "INVOKE", Endpoint: CustomerMembershipsCoreEndpoint, Body: valid}
		mutate(&in)
		_, err := invoker.InvokeCoreCapability(f.ctx, in)
		require.ErrorIs(t, err, ErrVerificationInvalidArgument)
	}
}
