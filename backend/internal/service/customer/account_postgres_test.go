package customer_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	capsvc "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 使用真实 PostgreSQL 的 UUID 类型；所有测试表和数据都位于回滚事务的隔离 schema。
func TestCustomerCreationPostgresNullablePrimaryContact(t *testing.T) {
	configPath := os.Getenv("POWERX_CUSTOMER_TEST_CONFIG")
	if configPath == "" {
		t.Skip("set POWERX_CUSTOMER_TEST_CONFIG to run PostgreSQL regression")
	}
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.Equal(t, "postgres", db.Dialector.Name())
	tx := db.Begin()
	require.NoError(t, tx.Error)
	previousSchema := coremodel.PowerXSchema
	testSchema := "customer_uuid_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		require.NoError(t, tx.Rollback().Error)
		coremodel.PowerXSchema = previousSchema
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA "`+testSchema+`"`).Error)
	coremodel.PowerXSchema = testSchema
	require.NoError(t, tx.AutoMigrate(&modelcustomer.Account{}, &modelcustomer.TenantMembership{}, &modelcustomer.Contact{}, &modelcustomer.AuthIdentity{}, &modelaudit.AuditEvent{}))
	ctx := context.Background()
	tenantUUID := uuid.NewString()

	// 即使现有数据库列没有 DEFAULT，新模型首次写入也必须得到 SQL NULL。
	require.NoError(t, tx.Exec(`ALTER TABLE "`+testSchema+`".customer_tenant_memberships ALTER COLUMN primary_contact_uuid DROP DEFAULT`).Error)
	account := &modelcustomer.Account{Type: "person", DisplayName: "待绑定客户", Status: "active"}
	membership := &modelcustomer.TenantMembership{Status: "active"}
	require.NoError(t, customerrepo.NewAccountRepository(tx).CreateWithMembership(ctx, tenantUUID, account, membership))
	var pending int64
	require.NoError(t, tx.Model(membership).Where("uuid = ? AND primary_contact_uuid IS NULL", membership.UUID).Count(&pending).Error)
	require.EqualValues(t, 1, pending)

	service := customersvc.NewAccountService(tx)
	for _, kind := range []string{"", "person", "company"} {
		input := customersvc.CreateAccountInput{TenantUUID: tenantUUID, Type: kind, DisplayName: "PG " + kind, PrimaryEmail: "pg@example.test"}
		if kind == "company" {
			input.PrimaryContact = &customersvc.PrimaryContactInput{DisplayName: "公司联系人", Email: "contact@example.test"}
		}
		created, err := service.Create(ctx, input)
		require.NoError(t, err)
		require.NotEmpty(t, created.PrimaryContactUUID)
		expectedType := kind
		if expectedType == "" {
			expectedType = "person"
		}
		require.Equal(t, expectedType, created.Type)
		var stored modelcustomer.TenantMembership
		require.NoError(t, tx.Where("uuid = ?", created.MembershipUUID).First(&stored).Error)
		require.Equal(t, created.PrimaryContactUUID, stored.PrimaryContactUUID)
		var contact modelcustomer.Contact
		require.NoError(t, tx.Where("uuid = ? AND customer_uuid = ? AND tenant_uuid = ?", stored.PrimaryContactUUID, created.UUID, tenantUUID).First(&contact).Error)
		identity := modelcustomer.AuthIdentity{CustomerUUID: created.UUID, Provider: "test", ProviderSubject: uuid.NewString(), Email: "login@example.test", Status: "active"}
		require.NoError(t, tx.Create(&identity).Error)
		invoker := customersvc.NewCapabilityInvoker(service)
		invoke := func(tenant string, body map[string]any) (map[string]any, error) {
			return invoker.InvokeCoreCapability(ctx, capsvc.CoreCapabilityInvokeInput{CapabilityID: customersvc.CustomerAccountsServiceManageCapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: "core://customer/accounts", Body: body})
		}
		if kind == "" {
			require.NoError(t, tx.Model(&modelcustomer.Account{}).Where("uuid = ?", created.UUID).Update("type", "").Error)
			require.NoError(t, tx.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", created.MembershipUUID).Update("primary_contact_uuid", nil).Error)
			readOnly, err := service.Get(ctx, tenantUUID, created.UUID)
			require.NoError(t, err)
			require.Empty(t, readOnly.Type)
			require.Empty(t, readOnly.PrimaryContactUUID)
		}
		body := map[string]any{"operation": "update", "customer_uuid": created.UUID, "display_name": "Updated", "primary_phone": "", "nickname": "Nick", "status": "suspended"}
		out, err := invoke(tenantUUID, body)
		require.NoError(t, err)
		updated := out["item"].(customerrepo.AccountRow)
		require.Equal(t, "Updated", updated.DisplayName)
		require.Equal(t, created.PrimaryEmail, updated.PrimaryEmail)
		require.Equal(t, created.Type, updated.Type)
		require.Equal(t, created.PrimaryContactUUID, updated.PrimaryContactUUID)
		require.Equal(t, "suspended", updated.MemberStatus)
		_, err = invoke(tenantUUID, map[string]any{"operation": "update", "customer_uuid": created.UUID, "nickname": "", "primary_email": ""})
		require.NoError(t, err)
		reread, err := service.Get(ctx, tenantUUID, created.UUID)
		require.NoError(t, err)
		require.Empty(t, reread.Nickname)
		require.Empty(t, reread.PrimaryEmail)
		require.Equal(t, "Updated", reread.DisplayName)
		var contactAfter modelcustomer.Contact
		require.NoError(t, tx.Where("uuid = ?", contact.UUID).First(&contactAfter).Error)
		require.Equal(t, contact, contactAfter)
		var identityAfter modelcustomer.AuthIdentity
		require.NoError(t, tx.Where("uuid = ?", identity.UUID).First(&identityAfter).Error)
		require.Equal(t, identity.Email, identityAfter.Email)
		_, err = invoke(uuid.NewString(), body)
		require.ErrorIs(t, err, customersvc.ErrCustomerAccountNotFound)
		_, err = invoke(tenantUUID, map[string]any{"operation": "update", "customer_uuid": "invalid", "display_name": "X"})
		require.ErrorIs(t, err, customersvc.ErrCustomerAccountInvalidArgument)
	}

	var before, after int64
	require.NoError(t, tx.Model(&modelcustomer.Account{}).Count(&before).Error)
	_, err = service.Create(ctx, customersvc.CreateAccountInput{TenantUUID: tenantUUID, Type: "person", DisplayName: "回滚客户", PrimaryEmail: "invalid-email"})
	require.ErrorIs(t, err, customersvc.ErrContactInvalidArgument)
	require.NoError(t, tx.Model(&modelcustomer.Account{}).Count(&after).Error)
	require.Equal(t, before, after)

	// 外部身份首次解析也会先插入未绑定 Contact 的 membership。
	provider, err := customerrepo.ExternalIdentityProviderKey("com.powerx.test.pg")
	require.NoError(t, err)
	resolved, err := customerrepo.NewAccountRepository(tx).ResolveOrCreateExternalIdentity(ctx, customerrepo.ExternalIdentityInput{TenantUUID: tenantUUID, ProviderKey: provider, ProviderPluginID: "com.powerx.test.pg", ProviderSubject: "pg-customer", DisplayName: "PG 身份客户"})
	require.NoError(t, err)
	require.NotEmpty(t, resolved.PrimaryContactUUID)
}
