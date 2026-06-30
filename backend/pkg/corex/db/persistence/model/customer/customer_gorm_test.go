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
