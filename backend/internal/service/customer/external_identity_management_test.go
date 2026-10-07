package customer

import (
	"context"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/stretchr/testify/require"
)

func TestIdentityManagementRejectsProxyFieldsAndOperationEscalation(t *testing.T) {
	invoker := NewCapabilityInvoker(NewAccountService(newCustomerServiceTestDB(t)))
	for _, field := range []string{"tenant_uuid", "plugin_id", "provider", "actor", "headers", "endpoint"} {
		_, err := invoker.InvokeCoreCapability(context.Background(), cap.CoreCapabilityInvokeInput{CapabilityID: CustomerExternalIdentitiesReadCapabilityID, Method: "INVOKE", Endpoint: "core://customer/external-identities", Body: map[string]any{"operation": "lookup", "provider_subject": "shop:test:customer:1", field: "forbidden"}})
		require.ErrorIs(t, err, ErrCustomerAccountInvalidArgument)
	}
	require.ErrorIs(t, validateIdentityManagementRequest(ExternalIdentityRequest{Operation: "bind", CustomerUUID: "11111111-1111-4111-8111-111111111111", ProviderSubject: "shop:test:customer:1"}, false), ErrCustomerAccountInvalidArgument)
	require.ErrorIs(t, validateIdentityManagementRequest(ExternalIdentityRequest{Operation: "lookup", ProviderSubject: "unscoped-id"}, false), ErrCustomerAccountInvalidArgument)
	require.Equal(t, "shop:example.myshopify.com:customer:gid://shopify/Customer/42", ShopifyExternalIdentitySubject("example.myshopify.com", "gid://shopify/Customer/42"))
}
