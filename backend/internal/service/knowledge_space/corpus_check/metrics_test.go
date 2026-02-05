package corpus_check

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecommendStrategyPackage_KG(t *testing.T) {
	key, _, _, _ := recommendStrategyPackage(10, 0.0, 0.0, 0.35, 0.1)
	require.Equal(t, "K_kg", key)
}

func TestRecommendStrategyPackage_CRAG(t *testing.T) {
	key, _, _, _ := recommendStrategyPackage(10, 0.45, 0.0, 0.0, 0.1)
	require.Equal(t, "O_crag", key)
}

func TestConstrainStrategyPackageToCatalog(t *testing.T) {
	t.Setenv("PX_SCENE_STRATEGY_CATALOG_PATH", filepath.Join("..", "..", "..", "..", "config", "knowledge", "scene_strategy_catalog.yaml"))
	cat := loadCatalog()
	require.NotNil(t, cat)

	key, _, scenes, profile := constrainStrategyPackageToCatalog(cat, "K_kg")
	require.Equal(t, "K_kg", key)
	require.Contains(t, scenes, "sql_kg")
	require.Equal(t, "p3_kg_strong", profile)
}
