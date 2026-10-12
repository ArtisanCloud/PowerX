package iam

import (
	"context"
	"testing"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestUpgradePermissionCatalogPreservesUUIDStatusEffectAndUserDescription(t *testing.T) {
	db := setupPermissionRepoTestDB(t)
	repo := NewPermissionRepository(db)
	existing := model.Permission{Module: "core", Resource: "source", Action: "read", Source: "corex.platform", Status: "deprecated", Effect: "deny", Description: "User retained description", AllowAPIKey: false, Meta: datatypes.JSON(`{"old":true}`)}
	require.NoError(t, db.Create(&existing).Error)
	updated := model.Permission{Module: "core", Resource: "source", Action: "read", Source: "corex.platform", Status: "active", Effect: "allow", Description: "New platform description", AllowAPIKey: true, Meta: datatypes.JSON(`{"api_key_explicit":true}`)}
	require.NoError(t, repo.UpsertUpgradeCatalog(context.Background(), []model.Permission{updated}))
	var got model.Permission
	require.NoError(t, db.First(&got, existing.ID).Error)
	require.Equal(t, existing.PermissionUUID, got.PermissionUUID)
	require.Equal(t, existing.Status, got.Status)
	require.Equal(t, "deny", got.Effect)
	require.Equal(t, existing.Description, got.Description)
	require.True(t, got.AllowAPIKey)
	// 源版本删除过的候选权限仍占用唯一键；保留墓碑，不重新创建或恢复授权。
	require.NoError(t, db.Delete(&got).Error)
	require.NoError(t, repo.UpsertUpgradeCatalog(context.Background(), []model.Permission{updated}))
	var deleted model.Permission
	require.NoError(t, db.Unscoped().First(&deleted, existing.ID).Error)
	require.True(t, deleted.DeletedAt.Valid)
	require.Equal(t, existing.PermissionUUID, deleted.PermissionUUID)
	updated.Source = "another-owner"
	require.ErrorContains(t, repo.UpsertUpgradeCatalog(context.Background(), []model.Permission{updated}), "UPGRADE_PERMISSION_SOURCE_CONFLICT")
}
