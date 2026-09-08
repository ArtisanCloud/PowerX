package customer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCapabilityInvokerResolvesOnlySTSPluginScopedExternalIdentity(t *testing.T) {
	db := newCustomerServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db))
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), "11111111-1111-1111-1111-111111111111"), &reqctx.CoreXClaims{
		TenantUUID: "11111111-1111-1111-1111-111111111111",
		PluginID:   "com.powerx.plugins.channel-a",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	})

	result, err := invoker.InvokeCoreCapability(ctx, capabilityregistry.CoreCapabilityInvokeInput{
		CapabilityID: CustomerExternalIdentitiesResolveCapabilityID,
		TenantUUID:   "11111111-1111-1111-1111-111111111111",
		Method:       "INVOKE",
		Endpoint:     customerExternalIdentityResolveEndpoint,
		Body: map[string]interface{}{
			"provider_subject": "channel-user-1",
			"display_name":     "客户甲",
		},
	})
	require.NoError(t, err)
	item, ok := result["item"].(ExternalIdentityResolveResult)
	require.True(t, ok)
	require.NotEmpty(t, item.CustomerUUID)
	require.NotEmpty(t, item.MembershipUUID)
	require.Equal(t, "客户甲", item.DisplayName)

	_, err = invoker.InvokeCoreCapability(ctx, capabilityregistry.CoreCapabilityInvokeInput{
		CapabilityID: CustomerExternalIdentitiesResolveCapabilityID,
		TenantUUID:   "11111111-1111-1111-1111-111111111111",
		Method:       "INVOKE",
		Endpoint:     customerExternalIdentityResolveEndpoint,
		Body: map[string]interface{}{
			"provider":         "wechat",
			"email":            "must-not-be-stored@example.invalid",
			"phone":            "13800000000",
			"provider_subject": "channel-user-2",
			"display_name":     "客户乙",
		},
	})
	require.ErrorIs(t, err, customerrepo.ErrExternalIdentityRequired)
}

type customerRoundTripper func(*http.Request) (*http.Response, error)

func (fn customerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestCustomerAuthUsesCoreShopifyVerifierAndAuditsOnlyIdentifierHash(t *testing.T) {
	db := newCustomerServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&settingmodel.PluginInstanceConfig{}))

	const (
		tenantUUID = "11111111-1111-1111-1111-111111111111"
		pluginID   = "com.powerx.plugins.shop"
		credential = "customer-access-token-must-not-be-audited"
	)
	require.NoError(t, db.Create(&settingmodel.PluginInstanceConfig{
		TenantUUID: tenantUUID,
		PluginID:   pluginID,
		Key:        ShopifyStorefrontConfigKey,
		Enabled:    true,
		ValueJSON:  datatypes.JSON([]byte(`{"shop_domain":"store.myshopify.com","storefront_access_token":"core-owned-storefront-token"}`)),
	}).Error)

	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantUUID), &reqctx.CoreXClaims{
		TenantUUID: tenantUUID,
		PluginID:   pluginID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	})
	tokens := NewCustomerTokenService(db, []byte("customer-test-secret"), "powerx-auth", time.Hour)
	service := NewCustomerAuthService(db, tokens)
	service.client = &http.Client{Transport: customerRoundTripper(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "store.myshopify.com", req.URL.Host)
		require.Equal(t, "core-owned-storefront-token", req.Header.Get("X-Shopify-Storefront-Access-Token"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), credential)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":{"customer":{"id":"gid://shopify/Customer/42","displayName":"已由 Core 校验的客户"}}}`)),
		}, nil
	})}

	pair, membership, err := service.RegisterShopify(ctx, tenantUUID, credential)
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.Equal(t, tenantUUID, membership.TenantUUID)
	require.NoError(t, service.AllowAttempt(ctx, tenantUUID, pluginID, ShopifyStorefrontChannel, credential, "127.0.0.1"))
	service.RecordAttempt(ctx, tenantUUID, pluginID, ShopifyStorefrontChannel, credential, "127.0.0.1", "customer.auth.register", "", true)

	var event modelcustomer.LoginEvent
	require.NoError(t, db.Last(&event).Error)
	require.Equal(t, customerIdentifierHash(credential), event.IdentifierHash)
	require.NotEqual(t, credential, event.IdentifierHash)
	var identity modelcustomer.AuthIdentity
	require.NoError(t, db.Where("provider_subject = ?", "shop:store.myshopify.com:customer:gid://shopify/Customer/42").First(&identity).Error)
	require.NotNil(t, identity.VerifiedAt, "只有 Core verifier 成功后才允许标记已验证")

	for i := 0; i < 10; i++ {
		service.RecordAttempt(ctx, tenantUUID, pluginID, ShopifyStorefrontChannel, credential, "127.0.0.2", "customer.auth.login", "CUSTOMER_CREDENTIAL_INVALID", false)
	}
	require.ErrorIs(t, service.AllowAttempt(ctx, tenantUUID, pluginID, ShopifyStorefrontChannel, credential, "127.0.0.2"), ErrCustomerForbidden)
}

func TestCapabilityInvokerRejectsExternalIdentityPII(t *testing.T) {
	db := newCustomerServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db))
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), "11111111-1111-1111-1111-111111111111"), &reqctx.CoreXClaims{
		TenantUUID: "11111111-1111-1111-1111-111111111111",
		PluginID:   "com.powerx.plugins.channel-a",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	})
	for _, field := range []string{"email", "phone"} {
		t.Run(field, func(t *testing.T) {
			_, err := invoker.InvokeCoreCapability(ctx, capabilityregistry.CoreCapabilityInvokeInput{
				CapabilityID: CustomerExternalIdentitiesResolveCapabilityID,
				TenantUUID:   "11111111-1111-1111-1111-111111111111",
				Method:       "INVOKE",
				Endpoint:     customerExternalIdentityResolveEndpoint,
				Body: map[string]interface{}{
					"provider_subject": "channel-user-1",
					"display_name":     "客户甲",
					field:              "forbidden",
				},
			})
			require.ErrorIs(t, err, customerrepo.ErrExternalIdentityRequired)
		})
	}
}

func TestCapabilityInvokerRejectsNonSTSExternalIdentityActor(t *testing.T) {
	db := newCustomerServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db))
	_, err := invoker.InvokeCoreCapability(context.Background(), capabilityregistry.CoreCapabilityInvokeInput{
		CapabilityID: CustomerExternalIdentitiesResolveCapabilityID,
		TenantUUID:   "11111111-1111-1111-1111-111111111111",
		Method:       "INVOKE",
		Endpoint:     customerExternalIdentityResolveEndpoint,
		Body: map[string]interface{}{
			"provider_subject": "channel-user-1",
			"display_name":     "客户甲",
		},
	})
	require.ErrorIs(t, err, customerrepo.ErrExternalIdentityServiceActorInvalid)
}

func TestCapabilityInvokerRejectsInvalidSTSExternalIdentityActor(t *testing.T) {
	db := newCustomerServiceTestDB(t)
	invoker := NewCapabilityInvoker(NewAccountService(db))
	for _, claims := range []*reqctx.CoreXClaims{
		{
			TenantUUID: "11111111-1111-1111-1111-111111111111", PluginID: "com.powerx.plugins.channel-a",
			RegisteredClaims: jwt.RegisteredClaims{Issuer: "another-issuer", Audience: jwt.ClaimStrings{"powerx:api"}},
		},
		{
			TenantUUID: "11111111-1111-1111-1111-111111111111", PluginID: "com.powerx.plugins.channel-a",
			RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"another-audience"}},
		},
	} {
		ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), "11111111-1111-1111-1111-111111111111"), claims)
		_, err := invoker.InvokeCoreCapability(ctx, capabilityregistry.CoreCapabilityInvokeInput{
			CapabilityID: CustomerExternalIdentitiesResolveCapabilityID,
			TenantUUID:   "11111111-1111-1111-1111-111111111111",
			Method:       "INVOKE",
			Endpoint:     customerExternalIdentityResolveEndpoint,
			Body:         map[string]interface{}{"provider_subject": "channel-user-1", "display_name": "客户甲"},
		})
		require.ErrorIs(t, err, customerrepo.ErrExternalIdentityServiceActorInvalid)
	}
}

func newCustomerServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
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
		`CREATE TABLE main.customer_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			customer_uuid TEXT NOT NULL, tenant_uuid TEXT, membership_uuid TEXT, session_family_uuid TEXT NOT NULL,
			access_token_jti TEXT UNIQUE, refresh_token_hash TEXT, source TEXT NOT NULL, issued_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL, revoked_at DATETIME, metadata TEXT
		)`,
		`CREATE TABLE main.customer_login_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			tenant_uuid TEXT, customer_uuid TEXT, identity_provider TEXT, plugin_id TEXT, channel TEXT, identifier_hash TEXT,
			event_type TEXT NOT NULL, ok BOOLEAN NOT NULL, error_code TEXT, ip TEXT, user_agent TEXT, trace_id TEXT, metadata TEXT
		)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	return db
}
