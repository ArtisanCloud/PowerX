package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"gorm.io/gorm"
)

// EnsureWorkflowDefinitionPackColumnsMigration brings existing workflow
// definition tables up to the current Workflow Pack contract.
func EnsureWorkflowDefinitionPackColumnsMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	switch db.Dialector.Name() {
	case "postgres":
		return ensureWorkflowDefinitionPackColumnsPostgres(db)
	case "sqlite":
		return ensureWorkflowDefinitionPackColumnsSQLite(db)
	}

	fields := []struct {
		field  string
		column string
	}{
		{field: "InputSchema", column: "input_schema"},
		{field: "WorkflowPackKey", column: "workflow_pack_key"},
		{field: "SourceType", column: "source_type"},
		{field: "Checksum", column: "checksum"},
	}

	model := &modelworkflow.WorkflowDefinition{}
	for _, item := range fields {
		if db.Migrator().HasColumn(model, item.column) {
			continue
		}
		if err := db.Migrator().AddColumn(model, item.field); err != nil {
			return fmt.Errorf("add workflow_definitions.%s failed: %w", item.column, err)
		}
		if !db.Migrator().HasColumn(model, item.column) {
			return fmt.Errorf("workflow_definitions.%s still missing after migration", item.column)
		}
	}

	return nil
}

func ensureWorkflowDefinitionPackColumnsPostgres(db *gorm.DB) error {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		schema = "public"
	}
	tableRef := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent(coremodel.TableWorkflowDefinitions))
	statements := []string{
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS input_schema jsonb NOT NULL DEFAULT '{}'::jsonb`, tableRef),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS workflow_pack_key varchar(128)`, tableRef),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS source_type varchar(32) NOT NULL DEFAULT 'manual'`, tableRef),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS checksum varchar(128)`, tableRef),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_workflow_definitions_pack_key ON %s (workflow_pack_key)`, tableRef),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_workflow_definitions_source_type ON %s (source_type)`, tableRef),
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure workflow definition pack columns failed: %w", err)
		}
	}
	for _, column := range []string{"input_schema", "workflow_pack_key", "source_type", "checksum"} {
		var count int64
		if err := db.Raw(
			`SELECT COUNT(1)
			   FROM information_schema.columns
			  WHERE table_schema = ?
			    AND table_name = ?
			    AND column_name = ?`,
			schema,
			coremodel.TableWorkflowDefinitions,
			column,
		).Scan(&count).Error; err != nil {
			return fmt.Errorf("verify workflow_definitions.%s failed: %w", column, err)
		}
		if count != 1 {
			return fmt.Errorf("workflow_definitions.%s still missing after migration", column)
		}
	}
	return nil
}

func ensureWorkflowDefinitionPackColumnsSQLite(db *gorm.DB) error {
	columns, err := sqliteTableColumns(db)
	if err != nil {
		return err
	}
	statements := map[string]string{
		"input_schema":      `ALTER TABLE public.workflow_definitions ADD COLUMN input_schema text NOT NULL DEFAULT '{}'`,
		"workflow_pack_key": `ALTER TABLE public.workflow_definitions ADD COLUMN workflow_pack_key text`,
		"source_type":       `ALTER TABLE public.workflow_definitions ADD COLUMN source_type text NOT NULL DEFAULT 'manual'`,
		"checksum":          `ALTER TABLE public.workflow_definitions ADD COLUMN checksum text`,
	}
	for _, column := range []string{"input_schema", "workflow_pack_key", "source_type", "checksum"} {
		if columns[column] {
			continue
		}
		if err := db.Exec(statements[column]).Error; err != nil {
			return fmt.Errorf("add workflow_definitions.%s failed: %w", column, err)
		}
	}
	columns, err = sqliteTableColumns(db)
	if err != nil {
		return err
	}
	for _, column := range []string{"input_schema", "workflow_pack_key", "source_type", "checksum"} {
		if !columns[column] {
			return fmt.Errorf("workflow_definitions.%s still missing after migration", column)
		}
	}
	return nil
}

func sqliteTableColumns(db *gorm.DB) (map[string]bool, error) {
	rows, err := db.Raw("PRAGMA public.table_info(workflow_definitions)").Rows()
	if err != nil {
		return nil, fmt.Errorf("inspect sqlite workflow_definitions columns failed: %w", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan sqlite workflow_definitions column failed: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect sqlite workflow_definitions columns failed: %w", err)
	}
	return columns, nil
}
