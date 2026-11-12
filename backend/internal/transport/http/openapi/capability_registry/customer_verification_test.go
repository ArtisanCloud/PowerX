package capability_registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

type verificationHTTPDelegate struct{ invoker cap.CoreCapabilityInvoker }

func (d *verificationHTTPDelegate) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	return d.invoker.InvokeCoreCapability(ctx, in)
}

func newVerificationHTTPFixture(t *testing.T) (*invocationHTTPFixture, string) {
	t.Helper()
	delegate := &verificationHTTPDelegate{}
	f := newInvocationHTTPFixture(t, false, delegate)
	delegate.invoker = customersvc.NewCapabilityInvoker(customersvc.NewAccountService(f.db))
	f.capID = customersvc.CustomerMembershipsServiceReadCapabilityID
	const scope = "_scope.customer.memberships.service_read"
	seedAPIKeyCatalogCapability(t, f.db, f.key, f.capID, scope)
	seedAPIKeyCatalogCapability(t, f.db, f.key, cap.GrantStatusCapabilityID, "_scope.capabilities.grant_status.read")
	// 使用正式 YAML 生成的权限，不能用测试专用宽权限代替服务 grant。
	_, source, _, _ := runtime.Caller(0)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../../config/platform_capabilities")))
	rows, err := apikeypermissions.BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	matched := 0
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] != f.capID {
			continue
		}
		require.True(t, row.AllowAPIKey)
		require.Equal(t, true, meta["api_key_explicit"])
		require.NoError(t, f.db.Where("resource=?", f.capID).Delete(&im.Permission{}).Error)
		require.NoError(t, f.db.Create(&row).Error)
		matched++
	}
	require.Equal(t, 1, matched)
	require.NoError(t, f.db.Model(&gw.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid=? AND scope=?", f.key.UUID, scope).Updates(map[string]any{"resource_type": "capability", "resource_pattern": "customer_memberships_service_read"}).Error)
	protocols, err := json.Marshal([]cm.ProtocolBinding{{Channel: "core_internal", Endpoint: customersvc.CustomerMembershipsCoreEndpoint, Method: "INVOKE", ActorContext: "service_actor", AuthType: "api_key_or_sts", ResourceScope: "tenant", SchemaRef: "docs/contracts/customer-membership-service-verification.openapi.yaml#/components/schemas/VerifyCustomerRequest"}})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", f.capID).Update("protocols", datatypes.JSON(protocols)).Error)
	policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
	require.NoError(t, f.db.Create(&policy).Error)
	var registration cm.CapabilityRegistration
	require.NoError(t, f.db.Where("capability_id=?", f.capID).First(&registration).Error)
	require.NoError(t, f.db.Model(&registration).Update("routing_policy_id", policy.UUID).Error)
	require.NoError(t, f.db.Create(&cm.AdapterEndpoint{RegistrationID: registration.ID, CapabilityID: f.capID, TenantUUID: apiKeyCatalogTestTenant, AdapterID: "verify", TransportType: "core_internal", Endpoint: customersvc.CustomerMembershipsCoreEndpoint, IsActive: true, Weight: 100, Labels: datatypes.JSON(`{"method":"INVOKE","actor_context":"service_actor"}`)}).Error)
	for _, sql := range []string{
		`CREATE TABLE main.customer_accounts (uuid TEXT PRIMARY KEY,status TEXT,updated_at DATETIME,deleted_at DATETIME)`,
		`CREATE TABLE main.customer_tenant_memberships (uuid TEXT PRIMARY KEY,tenant_uuid TEXT,customer_uuid TEXT,primary_contact_uuid TEXT,status TEXT,expires_at DATETIME,updated_at DATETIME,deleted_at DATETIME)`,
		`CREATE TABLE main.customer_contacts (uuid TEXT PRIMARY KEY,tenant_uuid TEXT,customer_uuid TEXT,status TEXT,updated_at DATETIME,deleted_at DATETIME)`,
	} {
		require.NoError(t, f.db.Exec(sql).Error)
	}
	customer, member, contact := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	require.NoError(t, f.db.Exec("INSERT INTO main.customer_accounts(uuid,status,updated_at) VALUES(?,?,?)", customer, "active", now).Error)
	require.NoError(t, f.db.Exec("INSERT INTO main.customer_tenant_memberships(uuid,tenant_uuid,customer_uuid,primary_contact_uuid,status,updated_at) VALUES(?,?,?,?,?,?)", member, apiKeyCatalogTestTenant, customer, contact, "active", now).Error)
	require.NoError(t, f.db.Exec("INSERT INTO main.customer_contacts(uuid,tenant_uuid,customer_uuid,status,updated_at) VALUES(?,?,?,?,?)", contact, apiKeyCatalogTestTenant, customer, "active", now).Error)
	return f, customer
}

func verificationEnvelope(f *invocationHTTPFixture, customer string) map[string]any {
	return map[string]any{"capability_id": f.capID, "preferred_protocol": "core_internal", "payload": map[string]any{"body": map[string]any{"operation": "verify_customer", "schema_version": 1, "customer_uuid": customer}}}
}

func TestCustomerVerificationHTTPAndAPIKeyRevocation(t *testing.T) {
	f, customer := newVerificationHTTPFixture(t)
	envelope := verificationEnvelope(f, customer)
	w := f.request(t, "/api/v1/tenant/invocations", envelope)
	require.Equal(t, 200, w.Code, w.Body.String())
	var result struct {
		Data struct {
			TraceID string `json:"trace_id"`
			Payload struct {
				Item customersvc.CustomerVerification `json:"item"`
			} `json:"payload"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.NotEmpty(t, result.Data.TraceID)
	require.Equal(t, customer, result.Data.Payload.Item.CustomerUUID)
	require.Equal(t, apiKeyCatalogTestTenant, result.Data.Payload.Item.TenantUUID)
	require.EqualValues(t, 0, f.calls.Load(), "不得经 REST 代理执行")
	checkGrant := func(want string) {
		w := f.request(t, "/api/v1/tenant/capabilities:grant-status", map[string]any{"capability_ids": []string{f.capID}})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"status":"`+want+`"`)
	}
	checkGrant("granted")
	for _, field := range []string{"endpoint", "method", "headers"} {
		bad := verificationEnvelope(f, customer)
		bad["payload"].(map[string]any)[field] = "forbidden"
		w = f.request(t, "/api/v1/tenant/invocations", bad)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	for _, field := range []string{"tenant_uuid", "actor", "plugin_id"} {
		bad := verificationEnvelope(f, customer)
		bad["payload"].(map[string]any)["body"].(map[string]any)[field] = "forged"
		w = f.request(t, "/api/v1/tenant/invocations", bad)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	w = f.request(t, "/api/v1/tenant/invocations", verificationEnvelope(f, uuid.NewString()))
	require.Equal(t, 404, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "CUSTOMER_MEMBERSHIP_NOT_FOUND")
	require.NoError(t, f.db.Where("api_key_uuid=? AND scope=?", f.key.UUID, "_scope.customer.memberships.service_read").Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	checkGrant("not_granted")
	w = f.request(t, "/api/v1/tenant/invocations", envelope)
	require.Equal(t, 403, w.Code, w.Body.String())
}

func TestCustomerVerificationErrorsPreserveReasonAndStatus(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		reason string
	}{
		{customersvc.ErrVerificationInvalidArgument, 400, "CUSTOMER_VERIFICATION_INVALID_ARGUMENT"},
		{customersvc.ErrVerificationUnauthorized, 401, "CUSTOMER_VERIFICATION_UNAUTHORIZED"},
		{customersvc.ErrVerificationForbidden, 403, "CUSTOMER_VERIFICATION_FORBIDDEN"},
		{customersvc.ErrVerificationNotFound, 404, "CUSTOMER_MEMBERSHIP_NOT_FOUND"},
		{customersvc.ErrVerificationCustomerInactive, 409, "CUSTOMER_ACCOUNT_INACTIVE"},
		{customersvc.ErrVerificationMembershipInactive, 409, "CUSTOMER_MEMBERSHIP_INACTIVE"},
		{customersvc.ErrVerificationMembershipExpired, 409, "CUSTOMER_MEMBERSHIP_EXPIRED"},
		{customersvc.ErrVerificationPrimaryContactInvalid, 409, "CUSTOMER_PRIMARY_CONTACT_INVALID"},
		{customersvc.ErrVerificationUnavailable, 503, "CUSTOMER_VERIFICATION_UNAVAILABLE"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", nil)
			err := fmt.Errorf("private dependency details: %w", tc.err)
			respondInvocationFailure(ctx, err, "verification-trace")
			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.reason)
			require.NotContains(t, w.Body.String(), "private dependency details")
			require.Contains(t, w.Body.String(), "verification-trace")
		})
	}
}
