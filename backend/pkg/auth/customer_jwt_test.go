package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenerateCustomerAccessJWTIncludesCustomerClaimsAndJTI(t *testing.T) {
	const tenantUUID = "11111111-1111-1111-1111-111111111111"
	const customerUUID = "22222222-2222-2222-2222-222222222222"
	token, err := GenerateCustomerAccessJWT(tenantUUID, customerUUID, "powerx-auth", time.Minute, []byte("secret"))
	require.NoError(t, err)
	claims, err := ParseAndValidate(token, []byte("secret"), "powerx-auth", "customer")
	require.NoError(t, err)
	require.Equal(t, tenantUUID, claims.TenantUUID)
	require.Equal(t, customerUUID, claims.CustomerUUID)
	require.Equal(t, customerUUID, claims.Subject)
	require.NotEmpty(t, claims.ID)
}
