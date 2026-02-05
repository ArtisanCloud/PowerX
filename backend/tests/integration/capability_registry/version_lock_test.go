package capabilityregistryintegration

import (
	"context"
	"testing"

	toolstore "github.com/ArtisanCloud/PowerX/internal/agent/toolstore"
	"github.com/stretchr/testify/require"
)

// TestVersionLockRequiresManualUpgrade 模拟新 capabilities_hash 发布后需管理员确认才能切换。
func TestVersionLockRequiresManualUpgrade(t *testing.T) {
	ctx := context.Background()
	store := toolstore.NewVersionLockStore(toolstore.VersionLockStoreOptions{})

	const (
		tenantUUID   = "1c2d3e4f-1111-4a4a-9b9b-222233334444"
		capabilityID = "cap.version.locked"
		hashV1       = "hash-demo-v1"
		hashV2       = "hash-demo-v2"
	)

	require.NoError(t, store.Bind(ctx, tenantUUID, capabilityID, hashV1))

	// 初始 hash 匹配，应允许运行。
	require.NoError(t, store.Enforce(ctx, tenantUUID, capabilityID, hashV1))

	// 新 hash 发布但未确认前，应拒绝并提示升级。
	err := store.Enforce(ctx, tenantUUID, capabilityID, hashV2)
	require.ErrorIs(t, err, toolstore.ErrVersionUpgradeRequired)

	locked, ok := store.CurrentHash(ctx, tenantUUID, capabilityID)
	require.True(t, ok, "版本锁应记录当前绑定 hash")
	require.Equal(t, hashV1, locked)

	// 管理员确认后，新的 hash 应生效且旧 hash 被拒绝。
	require.NoError(t, store.ConfirmUpgrade(ctx, tenantUUID, capabilityID, hashV2))
	require.NoError(t, store.Enforce(ctx, tenantUUID, capabilityID, hashV2))
	require.ErrorIs(t, store.Enforce(ctx, tenantUUID, capabilityID, hashV1), toolstore.ErrVersionUpgradeRequired)

	lockedAfter, ok := store.CurrentHash(ctx, tenantUUID, capabilityID)
	require.True(t, ok)
	require.Equal(t, hashV2, lockedAfter)
}
