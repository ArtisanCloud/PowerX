package customer_test

import (
	"context"
	"encoding/json"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"gorm.io/datatypes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	svc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestExternalIdentityManagementPostgres(t *testing.T) {
	path := os.Getenv("POWERX_CUSTOMER_TEST_CONFIG")
	if path == "" {
		t.Skip("PostgreSQL configuration required")
	}
	cfg, err := config.Load(path)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := coremodel.PowerXSchema
	schema := "external_identity_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		coremodel.PowerXSchema = previous
	})
	require.NoError(t, db.AutoMigrate(&model.Account{}, &model.TenantMembership{}, &model.Contact{}, &model.AuthIdentity{}, &audit.AuditEvent{}, &capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &iam.Permission{}))
	for _, capID := range []string{svc.CustomerExternalIdentitiesReadCapabilityID, svc.CustomerExternalIdentitiesManageCapabilityID} {
		require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: capID, PluginID: "core", PluginVersion: "test", Title: capID, Status: "published", CapabilitiesHash: "test", ProtocolHash: "test"}).Error)
	}
	tenant := uuid.NewString()
	contextFor := func(tenant, plugin string) context.Context {
		client := uuid.NewString()
		allowed := []string{svc.CustomerExternalIdentitiesReadCapabilityID, svc.CustomerExternalIdentitiesManageCapabilityID}
		raw, _ := json.Marshal(map[string]any{"client_id": client, "allowed_capabilities": allowed})
		require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
		for _, capID := range allowed {
			require.NoError(t, db.Create(&capmodel.CapabilityRegistration{CapabilityID: capID, TenantUUID: tenant, ContractRef: "test", Status: "published", Version: uint64(time.Now().UnixNano()), RoutingPolicyID: uuid.New()}).Error)
		}
		return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}, Subject: "client:" + client, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	}
	ctx := contextFor(tenant, "com.powerx.test.identity")
	service := svc.NewAccountService(db)
	subject := "shop:example.myshopify.com:customer:gid://shopify/Customer/42"
	read := func(ctx context.Context, op, customer, subject string) map[string]any {
		out, e := service.ManageExternalIdentity(ctx, svc.CustomerExternalIdentitiesReadCapabilityID, svc.ExternalIdentityRequest{Operation: op, CustomerUUID: customer, ProviderSubject: subject})
		require.NoError(t, e)
		return out
	}
	require.Equal(t, false, read(ctx, "lookup", "", subject)["found"])
	var count int64
	require.NoError(t, db.Model(&model.Account{}).Count(&count).Error)
	require.Zero(t, count)
	input := svc.ExternalIdentityRequest{Operation: "create_and_bind", ProviderSubject: subject, Customer: &svc.ExternalCustomerProfile{PrimaryEmail: "only@example.test"}}
	// Independent PostgreSQL transactions race on one identity; exactly one aggregate wins.
	const parallel = 8
	results := make(chan svc.ExternalIdentityItem, parallel)
	failures := make(chan error, parallel)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := service.ManageExternalIdentity(ctx, svc.CustomerExternalIdentitiesManageCapabilityID, input)
			if e != nil {
				failures <- e
				return
			}
			results <- out["item"].(svc.ExternalIdentityItem)
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		require.NoError(t, e)
	}
	var first svc.ExternalIdentityItem
	for result := range results {
		if first.CustomerUUID == "" {
			first = result
		}
		require.Equal(t, first, result)
	}
	require.NotEmpty(t, first.CustomerUUID)
	require.NotEmpty(t, first.PrimaryContactUUID)
	for _, table := range []any{&model.Account{}, &model.TenantMembership{}, &model.Contact{}, &model.AuthIdentity{}} {
		require.NoError(t, db.Model(table).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
	var account model.Account
	require.NoError(t, db.Where("uuid = ?", first.CustomerUUID).First(&account).Error)
	require.Equal(t, "only@example.test", account.DisplayName)
	require.Empty(t, account.GivenName)
	require.Empty(t, account.FamilyName)
	var contact model.Contact
	require.NoError(t, db.Where("uuid = ?", first.PrimaryContactUUID).First(&contact).Error)
	require.Equal(t, "only@example.test", contact.DisplayName)
	require.Empty(t, contact.GivenName)
	require.Equal(t, true, read(ctx, "lookup", "", subject)["found"])
	require.Equal(t, false, read(contextFor(uuid.NewString(), "com.powerx.test.identity"), "lookup", "", subject)["found"])
	require.Equal(t, false, read(contextFor(tenant, "com.powerx.test.other"), "lookup", "", subject)["found"])
	require.EqualValues(t, 1, read(ctx, "list_by_customer", first.CustomerUUID, "")["total"])
	// API Key plugin identity is authoritative grant metadata, not caller payload.
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "fixture", KeyPrefix: "fixture", KeyHash: uuid.NewString(), Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	var grants []gw.IntegrationGatewayAPIKeyPermission
	for _, suffix := range []string{"service_read", "service_manage"} {
		action := "read"
		if suffix == "service_manage" {
			action = "manage"
		}
		capID := "com.corex.customer.external_identities." + suffix
		meta, _ := json.Marshal(map[string]any{"capability_id": capID, "api_key_explicit": true, "api_key": map[string]string{"scope": "_scope.customer.external_identities." + suffix, "action": action, "resource_type": "capability", "resource_pattern": "customer_external_identities_" + suffix, "effect": "allow"}})
		require.NoError(t, db.Create(&iam.Permission{Module: "customer", Resource: suffix, Action: action, Effect: "allow", AllowAPIKey: true, Status: iam.PermissionStatusActive, Meta: datatypes.JSON(meta)}).Error)
		grant := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.customer.external_identities." + suffix, Action: action, ResourceType: "capability", ResourcePattern: "customer_external_identities_" + suffix, Effect: "allow", PluginID: "com.powerx.test.identity"}
		require.NoError(t, db.Create(&grant).Error)
		grants = append(grants, grant)
	}
	keyCtx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	require.Equal(t, true, read(keyCtx, "lookup", "", subject)["found"])
	replay, err := service.ManageExternalIdentity(keyCtx, svc.CustomerExternalIdentitiesManageCapabilityID, input)
	require.NoError(t, err)
	require.Equal(t, first, replay["item"])
	require.NoError(t, db.Model(&grants[0]).Update("plugin_id", "").Error)
	_, err = service.ManageExternalIdentity(keyCtx, svc.CustomerExternalIdentitiesReadCapabilityID, svc.ExternalIdentityRequest{Operation: "lookup", ProviderSubject: subject})
	require.Error(t, err)
	require.NoError(t, db.Unscoped().Delete(&grants[1]).Error)
	_, err = service.ManageExternalIdentity(keyCtx, svc.CustomerExternalIdentitiesManageCapabilityID, input)
	require.Error(t, err)
	other, err := service.Create(ctx, svc.CreateAccountInput{TenantUUID: tenant, DisplayName: "Manual", PrimaryEmail: "only@example.test"})
	require.NoError(t, err)
	require.NotEqual(t, first.CustomerUUID, other.UUID)
	bind := svc.ExternalIdentityRequest{Operation: "bind", CustomerUUID: other.UUID, ProviderSubject: subject}
	_, err = service.ManageExternalIdentity(ctx, svc.CustomerExternalIdentitiesManageCapabilityID, bind)
	require.ErrorIs(t, err, svc.ErrExternalIdentityConflict)
	bind.ProviderSubject = "shop:example.myshopify.com:customer:another"
	for i := 0; i < 2; i++ {
		_, err = service.ManageExternalIdentity(ctx, svc.CustomerExternalIdentitiesManageCapabilityID, bind)
		require.NoError(t, err)
	}
	current, err := service.Get(ctx, tenant, other.UUID)
	require.NoError(t, err)
	require.Equal(t, "Manual", current.DisplayName)
	// Missing legacy type/contact references are not repaired by read or bind.
	require.NoError(t, db.Model(&model.Account{}).Where("uuid = ?", other.UUID).Update("type", "").Error)
	require.NoError(t, db.Model(&model.TenantMembership{}).Where("customer_uuid = ?", other.UUID).Update("primary_contact_uuid", nil).Error)
	read(ctx, "lookup", "", bind.ProviderSubject)
	read(ctx, "list_by_customer", other.UUID, "")
	bind.ProviderSubject = "shop:example.myshopify.com:customer:third"
	_, err = service.ManageExternalIdentity(ctx, svc.CustomerExternalIdentitiesManageCapabilityID, bind)
	require.NoError(t, err)
	current, err = service.Get(ctx, tenant, other.UUID)
	require.NoError(t, err)
	require.Empty(t, current.Type)
	require.Empty(t, current.PrimaryContactUUID)
	// Inject an identity INSERT failure after aggregate creation to verify full rollback.
	require.NoError(t, db.Exec(`ALTER TABLE "`+schema+`".customer_auth_identities ADD CONSTRAINT reject_fixture CHECK (provider_subject <> 'shop:test:customer:reject')`).Error)
	require.NoError(t, db.Model(&model.Account{}).Count(&count).Error)
	before := count
	input.ProviderSubject = "shop:test:customer:reject"
	_, err = service.ManageExternalIdentity(ctx, svc.CustomerExternalIdentitiesManageCapabilityID, input)
	require.Error(t, err)
	require.NoError(t, db.Model(&model.Account{}).Count(&count).Error)
	require.Equal(t, before, count)
}
