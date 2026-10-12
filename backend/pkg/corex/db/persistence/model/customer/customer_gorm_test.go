package customer

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// 不依赖数据库连接，但使用 PostgreSQL 方言检查真实 INSERT 列表。
func TestMembershipInsertOmitsUnsetPrimaryContactUUID(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	require.NoError(t, err)
	for _, contactUUID := range []string{"", "22222222-2222-4222-8222-222222222222"} {
		row := &TenantMembership{TenantUUID: "11111111-1111-4111-8111-111111111111", CustomerUUID: "33333333-3333-4333-8333-333333333333", PrimaryContactUUID: contactUUID, Status: "active"}
		result := db.Create(row)
		require.NoError(t, result.Error)
		columns := strings.SplitN(result.Statement.SQL.String(), " VALUES ", 2)[0]
		if contactUUID == "" {
			require.NotContains(t, columns, `"primary_contact_uuid"`, "未绑定 UUID 必须省略，交给 nullable 列保存 SQL NULL")
		} else {
			require.Contains(t, columns, `"primary_contact_uuid"`)
			require.Contains(t, result.Statement.Vars, contactUUID)
		}
	}
}

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
		{model: &ContactIdentity{}, columns: []string{"uuid", "tenant_uuid", "customer_uuid", "contact_uuid", "channel_dictionary_item_uuid", "external_subject", "status"}},
	} {
		parsed, err := schema.Parse(testCase.model, &sync.Map{}, schema.NamingStrategy{})
		require.NoError(t, err)
		for _, column := range testCase.columns {
			require.NotNil(t, parsed.LookUpField(column), "expected contact model column %s", column)
		}
	}
}
