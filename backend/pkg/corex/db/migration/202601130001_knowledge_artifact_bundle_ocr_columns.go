package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureKnowledgeArtifactBundleOCRColumns backfills OCR artifact URIs columns for existing installs.
// It MUST be idempotent.
func EnsureKnowledgeArtifactBundleOCRColumns(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		schema = "public"
	}
	tableRef := fmt.Sprintf(`"%s"."%s"`, schema, coremodel.TableKnowledgeArtifactBundles)

	stmts := []string{
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS ocr_page_images_uri text`, tableRef),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS ocr_raw_manifest_uri text`, tableRef),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS ocr_searchable_pdf_uri text`, tableRef),
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

