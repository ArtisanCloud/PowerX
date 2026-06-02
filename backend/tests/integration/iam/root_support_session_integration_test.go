package iamintegration

import (
	"testing"
	"time"

	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/stretchr/testify/require"
)

func TestRootSupportSessionPersistenceIntegration(t *testing.T) {
	fx := setupIAMFixture(t)

	targetTenantUUID := "22222222-2222-4222-8222-222222222222"
	tenant := &modeltenant.Tenant{
		Key:    "tenant-support-target",
		Name:   "Tenant Support Target",
		Status: 1,
		Type:   modeltenant.TenantTypeEnterprise,
		Plan:   modeltenant.TenantPlanFree,
	}
	tenant.UUID = mustUUID(targetTenantUUID)
	require.NoError(t, fx.DB.Create(tenant).Error)

	root := &modeliam.User{
		Email:       "root-support@example.com",
		DisplayName: "Root Support",
		Status:      modeliam.UserStatusActive,
		IsRoot:      true,
	}
	root.ID = 9001
	require.NoError(t, fx.DB.Create(root).Error)

	now := time.Now().UTC().Truncate(time.Second)
	insert := fx.DB.Exec(`
		INSERT INTO iam_root_support_sessions
			(root_user_id, target_tenant_uuid, reason, mode, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, root.ID, targetTenantUUID, "investigate tenant AI settings issue", "read_only", "active", now)
	require.NoError(t, insert.Error, "root support session migration must create iam_root_support_sessions")
	require.Equal(t, int64(1), insert.RowsAffected)

	var got struct {
		ID               uint64
		RootUserID       uint64
		TargetTenantUUID string
		Reason           string
		Mode             string
		Status           string
		StartedAt        time.Time
		EndedAt          *time.Time
	}
	require.NoError(t, fx.DB.Raw(`
		SELECT id, root_user_id, target_tenant_uuid, reason, mode, status, started_at, ended_at
		FROM iam_root_support_sessions
		WHERE root_user_id = ? AND target_tenant_uuid = ?
	`, root.ID, targetTenantUUID).Scan(&got).Error)
	require.NotZero(t, got.ID)
	require.Equal(t, root.ID, got.RootUserID)
	require.Equal(t, targetTenantUUID, got.TargetTenantUUID)
	require.Equal(t, "investigate tenant AI settings issue", got.Reason)
	require.Equal(t, "read_only", got.Mode)
	require.Equal(t, "active", got.Status)
	require.Nil(t, got.EndedAt)

	endedAt := now.Add(5 * time.Minute)
	update := fx.DB.Exec(`
		UPDATE iam_root_support_sessions
		SET status = ?, ended_at = ?
		WHERE id = ? AND status = ?
	`, "ended", endedAt, got.ID, "active")
	require.NoError(t, update.Error)
	require.Equal(t, int64(1), update.RowsAffected)

	var ended struct {
		Status  string
		EndedAt *time.Time
	}
	require.NoError(t, fx.DB.Raw(`
		SELECT status, ended_at
		FROM iam_root_support_sessions
		WHERE id = ?
	`, got.ID).Scan(&ended).Error)
	require.Equal(t, "ended", ended.Status)
	require.NotNil(t, ended.EndedAt)
	require.True(t, ended.EndedAt.Equal(endedAt), "ended_at must be persisted for audit")
}
