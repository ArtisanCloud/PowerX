package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureDevHotloadReloadTokenText widens dev hotload reload_token column to TEXT for JWT support.
func EnsureDevHotloadReloadTokenText(db *gorm.DB) error {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		schema = "public"
	}

	type columnInfo struct {
		DataType string `gorm:"column:data_type"`
	}
	const columnQuery = `
SELECT data_type
FROM information_schema.columns
WHERE table_schema = ?
  AND table_name = ?
  AND column_name = 'reload_token'
LIMIT 1;`
	var info columnInfo
	if err := db.Raw(columnQuery, schema, coremodel.TableDevHotloadSessions).Scan(&info).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if strings.EqualFold(info.DataType, "text") {
		return nil
	}

	tableRef := fmt.Sprintf(`"%s"."%s"`, schema, coremodel.TableDevHotloadSessions)
	alterSQL := fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN reload_token TYPE text`, tableRef)
	return db.Exec(alterSQL).Error
}
