package authorizationbaseline

import (
	"context"
	"testing"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBadRootCatalogPreservesStartupHistoryAndAllowsRevocation(t *testing.T) {
	for _, action := range []string{"disable_user", "revoke_role", "revoke_root", "disable_root_user"} {
		t.Run(action, func(t *testing.T) {
			db := bootstrapDB(t)
			root, ordinary, role, stale := rootRepairFixture(t, db)
			beforeRoot, segment := root.current(t)
			require.NoError(t, EnsureInitialized(db))
			require.ErrorIs(t, rootrole.CheckCatalog(db), rootrole.ErrCatalogInconsistent)
			afterStartup, afterSegment := root.current(t)
			require.Equal(t, beforeRoot, afterStartup)
			require.Equal(t, segment, afterSegment)
			// 严格目录同步仍拒绝；启动和无关成员管理没有悄悄修复旧 UUID。
			_, err := rootrole.Ensure(db)
			require.ErrorIs(t, err, rootrole.ErrCatalogInconsistent)
			switch action {
			case "disable_user":
				require.NoError(t, db.Model(&im.User{}).Where("uuid=?", ordinary.member.UserUUID).Update("status", 2).Error)
			case "revoke_role":
				require.NoError(t, db.Delete(&ordinary.binding).Error)
			case "revoke_root":
				require.NoError(t, db.Model(&im.User{}).Where("uuid=?", root.member.UserUUID).Update("is_root", false).Error)
			case "disable_root_user":
				require.NoError(t, db.Model(&im.User{}).Where("uuid=?", root.member.UserUUID).Update("status", 2).Error)
			}
			if action == "revoke_root" || action == "disable_root_user" {
				_, current := root.current(t)
				require.Equal(t, Denied, current.State)
			} else {
				_, current := ordinary.current(t)
				require.Equal(t, Denied, current.State)
				after, currentRoot := root.current(t)
				require.Equal(t, beforeRoot, after)
				require.Equal(t, segment, currentRoot)
				require.Equal(t, RootBypass, currentRoot.State)
			}
			var link im.RolePermission
			require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&link).Error)
			require.Equal(t, stale, link)
		})
	}
}

func TestRepairPlanRejectsNewlyAffectedRootHistory(t *testing.T) {
	checkRepairPlanRejectsNewlyAffectedRootHistory(t, bootstrapDB(t))
}

func TestPostgresRepairPlanRejectsNewlyAffectedRootHistory(t *testing.T) {
	checkRepairPlanRejectsNewlyAffectedRootHistory(t, rootCatalogPostgresDB(t))
}

func checkRepairPlanRejectsNewlyAffectedRootHistory(t *testing.T, db *gorm.DB) {
	t.Helper()
	root, ordinary, role, stale := rootRepairFixture(t, db)
	preview, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
	require.NoError(t, err)
	require.Len(t, preview.History, 1)
	// 模拟维护预览后旧版 Core 合法提交了身份及历史变化；仅写隔离库。
	pool := db.Statement.ConnPool.(*transactionPool)
	query := "UPDATE " + (&im.User{}).TableName() + " SET is_root=? WHERE uuid=?"
	if db.Dialector.Name() == "postgres" {
		query = "UPDATE " + (&im.User{}).TableName() + " SET is_root=$1 WHERE uuid=$2"
	}
	_, err = pool.ConnPool.ExecContext(context.Background(), query, true, ordinary.member.UserUUID)
	require.NoError(t, err)
	require.NoError(t, Reconcile(db, Micro(time.Now())))
	beforeNewRoot, _ := ordinary.current(t)
	afterPreview, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
	require.NoError(t, err)
	require.Len(t, afterPreview.History, 2)
	require.NotEqual(t, preview.Fingerprint, afterPreview.Fingerprint)
	_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
	require.ErrorIs(t, err, ErrRootRepairPlanConflict)
	var row im.RolePermission
	require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&row).Error)
	require.Equal(t, stale, row)
	afterRejected, _ := ordinary.current(t)
	require.Equal(t, beforeNewRoot, afterRejected)
	_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: afterPreview.Fingerprint, Operator: "fixture"})
	require.NoError(t, err)
	_, rootSegment := root.current(t)
	require.Equal(t, Allowed, rootSegment.State)
	afterAccepted, _ := ordinary.current(t)
	require.Equal(t, beforeNewRoot.FactRevision+1, afterAccepted.FactRevision)
}

func TestPostgresBadRootCatalogAllowsUserDeactivation(t *testing.T) {
	db := rootCatalogPostgresDB(t)
	root, ordinary, _, _ := rootRepairFixture(t, db)
	before, segment := root.current(t)
	require.NoError(t, EnsureInitialized(db))
	require.NoError(t, db.Model(&im.User{}).Where("uuid=?", ordinary.member.UserUUID).Update("status", 2).Error)
	_, decision := ordinary.current(t)
	require.Equal(t, Denied, decision.State)
	after, current := root.current(t)
	require.Equal(t, before, after)
	require.Equal(t, segment, current)
	require.ErrorIs(t, rootrole.CheckCatalog(db), rootrole.ErrCatalogInconsistent)
}

func TestRepairPlanPinsTrackingRevisionAndHistoricalSegments(t *testing.T) {
	for _, change := range []string{"stream_revision", "tracking_start", "historical_segment"} {
		t.Run(change, func(t *testing.T) {
			db := bootstrapDB(t)
			root, _, role, stale := rootRepairFixture(t, db)
			preview, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
			require.NoError(t, err)
			repeated, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
			require.NoError(t, err)
			require.Equal(t, preview.Fingerprint, repeated.Fingerprint)
			stream, segment := root.current(t)
			switch change {
			case "stream_revision":
				require.NoError(t, db.Model(&im.AuthorizationStream{}).Where("uuid=?", stream.UUID).Update("fact_revision", stream.FactRevision+1).Error)
			case "tracking_start":
				require.NoError(t, db.Model(&im.AuthorizationTracking{}).Where("tenant_uuid=?", root.tenant).Update("known_since", Micro(time.Now().Add(-48*time.Hour))).Error)
			case "historical_segment":
				require.NoError(t, db.Model(&im.AuthorizationSegment{}).Where("uuid=?", segment.UUID).Update("sources_hash", "fixture-history-change").Error)
			}
			_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
			require.ErrorIs(t, err, ErrRootRepairPlanConflict)
			var row im.RolePermission
			require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&row).Error)
			require.Equal(t, stale, row)
		})
	}
}
