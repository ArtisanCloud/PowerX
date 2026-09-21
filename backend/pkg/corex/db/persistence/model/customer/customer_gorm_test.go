package customer

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func TestAccountAutoMigrateIncludesBaseProfileColumns(t *testing.T) {
	parsed, err := schema.Parse(&Account{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)

	for _, column := range []string{"nickname", "given_name", "family_name"} {
		require.NotNil(t, parsed.LookUpField(column), "expected customer account column %s", column)
	}
}

func TestContactModelsExposeScopedUUIDFields(t *testing.T) {
	for _, testCase := range []struct {
		model   any
		columns []string
	}{
		{model: &Contact{}, columns: []string{"uuid", "tenant_uuid", "customer_uuid", "display_name", "roles", "tags", "status"}},
		{model: &ContactIdentity{}, columns: []string{"uuid", "tenant_uuid", "customer_uuid", "contact_uuid", "channel", "external_subject", "status"}},
	} {
		parsed, err := schema.Parse(testCase.model, &sync.Map{}, schema.NamingStrategy{})
		require.NoError(t, err)
		for _, column := range testCase.columns {
			require.NotNil(t, parsed.LookUpField(column), "expected contact model column %s", column)
		}
	}
}
