package migration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureWorkflowDefinitionPackColumnsMigrationAddsMissingColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:workflow-definition-pack-columns?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`ATTACH DATABASE ':memory:' AS public`).Error)

	require.NoError(t, db.Exec(`
		CREATE TABLE public.workflow_definitions (
			id integer primary key autoincrement,
			uuid text,
			tenant_uuid text not null,
			name text not null,
			description text,
			version integer not null default 1,
			status text not null default 'draft',
			step_graph text not null,
			default_retry_policy text,
			compensation_policy text,
			sla_policy text,
			metadata text,
			created_by text not null,
			published_at datetime,
			archived_at datetime,
			last_published_by text,
			last_change_note text,
			version_alias text,
			initial_context_schema text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
	`).Error)

	require.NoError(t, EnsureWorkflowDefinitionPackColumnsMigration(db))

	columns := sqliteColumns(t, db)
	require.Contains(t, columns, "input_schema")
	require.Contains(t, columns, "workflow_pack_key")
	require.Contains(t, columns, "source_type")
	require.Contains(t, columns, "checksum")
}

func sqliteColumns(t *testing.T, db *gorm.DB) map[string]bool {
	t.Helper()
	rows, err := db.Raw("PRAGMA public.table_info(workflow_definitions)").Rows()
	require.NoError(t, err)
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk))
		out[name] = true
	}
	require.NoError(t, rows.Err())
	return out
}
