package customer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountUpdateRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"display_name", ""}, {"display_name", strings.Repeat("名", 129)}, {"status", ""}, {"status", "bogus"}, {"primary_email", "bad"}, {"primary_phone", "abc"}, {"primary_phone", "+()-"}, {"avatar_url", "javascript:alert(1)"}, {"locale", "not_a_locale_!"}, {"timezone", "No/SuchZone"},
	} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			req, err := decodeCustomerAccountUpdateRequest(map[string]any{"operation": "update", "customer_uuid": "11111111-1111-4111-8111-111111111111", tc.name: tc.value})
			require.NoError(t, err)
			_, err = validateAccountPatch(req.AccountProfilePatch)
			require.ErrorIs(t, err, ErrCustomerAccountInvalidArgument)
		})
	}
	for _, field := range []string{"tenant_uuid", "actor", "type", "primary_contact_uuid", "headers", "endpoint", "Display_Name", "email", "password"} {
		_, err := decodeCustomerAccountUpdateRequest(map[string]any{"operation": "update", "customer_uuid": "uuid", field: "forbidden"})
		require.ErrorIs(t, err, ErrCustomerAccountInvalidArgument)
	}
	for _, value := range []any{nil, 42, true, []string{"name"}} {
		_, err := decodeCustomerAccountUpdateRequest(map[string]any{"operation": "update", "customer_uuid": "uuid", "nickname": value})
		require.ErrorIs(t, err, ErrCustomerAccountInvalidArgument)
	}
}

func TestAccountUpdatePreservesOmissionAndClearsOptionalFields(t *testing.T) {
	req, err := decodeCustomerAccountUpdateRequest(map[string]any{"operation": "update", "customer_uuid": "uuid", "primary_phone": "", "locale": "zh-CN", "timezone": "Asia/Shanghai"})
	require.NoError(t, err)
	require.Nil(t, req.DisplayName)
	fields, err := validateAccountPatch(req.AccountProfilePatch)
	require.NoError(t, err)
	require.Equal(t, "", fields["primary_phone"])
	require.NotContains(t, fields, "display_name")
	_, err = validateAccountPatch(AccountProfilePatch{})
	require.ErrorIs(t, err, ErrCustomerAccountInvalidArgument)
}
