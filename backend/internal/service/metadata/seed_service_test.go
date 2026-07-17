package metadata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSeedServiceRejectsMissingCanonicalDefinitions(t *testing.T) {
	path := writeSeedTestFile(t, `
version: 1
module: corex.metadata
dictionaries: []
taxonomies: []
resource_types: []
tags: []
`)
	seed, err := LoadSeedFile(path)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	if err := ValidateCanonicalSeedDefinitions(seed); !errors.Is(err, ErrCanonicalSeedMissing) {
		t.Fatalf("expected canonical seed missing, got %v", err)
	}
}

func TestSeedLoaderRejectsUnknownSchemaFields(t *testing.T) {
	path := writeSeedTestFile(t, `
version: 1
module: corex.metadata
unknown: true
`)
	if _, err := LoadSeedFile(path); err == nil {
		t.Fatalf("expected unknown seed field to fail")
	}
}

func TestSeedServiceExecutesResourceTypes(t *testing.T) {
	db := newSeedServiceTestDB(t)
	path := writeSeedTestFile(t, `
version: 1
module: corex.metadata
dictionaries:
  - namespace: corex.metadata.status
    name_i18n:
      zh-CN: 状态
    items:
      - code: enabled
        label_i18n:
          zh-CN: 启用
resource_types:
  - resource_type: metadata.demo_resource
    name_i18n:
      zh-CN: 演示资源
    binding_enabled: false
tags:
  - namespace: corex.metadata.demo
    resource_type: metadata.demo_resource
    code: sample
    label_i18n:
      zh-CN: 示例
`)
	svc, err := NewSeedService(SeedServiceOptions{DB: db, SeedPath: path})
	if err != nil {
		t.Fatalf("new seed service: %v", err)
	}
	result, _, err := svc.Execute(context.Background(), SeedExecutionInput{
		TenantUUID:                  uuid.New().String(),
		RequireCanonicalDefinitions: true,
	})
	if err != nil {
		t.Fatalf("execute seed: %v", err)
	}
	if result.DictionaryNamespaces != 1 || result.DictionaryItems != 1 || result.ResourceTypes != 1 || result.Tags != 1 {
		t.Fatalf("unexpected seed result: %+v", result)
	}
}

func TestSeedServiceRejectsUnresolvedTaxonomyParent(t *testing.T) {
	db := newSeedServiceTestDB(t)
	path := writeSeedTestFile(t, `
version: 1
module: corex.metadata
taxonomies:
  - namespace: corex.metadata.category
    name_i18n:
      zh-CN: 分类
    max_depth: 3
    nodes:
      - code: child
        parent_code: missing_parent
        label_i18n:
          zh-CN: 子节点
`)
	svc, err := NewSeedService(SeedServiceOptions{DB: db, SeedPath: path})
	if err != nil {
		t.Fatalf("new seed service: %v", err)
	}
	_, _, err = svc.Execute(context.Background(), SeedExecutionInput{TenantUUID: uuid.New().String()})
	if !errors.Is(err, ErrInvalidParentReference) {
		t.Fatalf("expected invalid parent reference, got %v", err)
	}
}

func writeSeedTestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seed.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	return path
}

func newSeedServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.DictionaryNamespace{}, &model.DictionaryItem{}, &model.Taxonomy{}, &model.TaxonomyNode{}, &model.ResourceType{}, &model.Tag{}); err != nil {
		t.Fatalf("migrate seed models: %v", err)
	}
	return db
}
