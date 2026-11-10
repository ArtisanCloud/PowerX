package authorizationbaseline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func rootRepairFixture(t *testing.T, db *gorm.DB) (fixture, fixture, im.Role, im.RolePermission) {
	return rootRepairFixtureCount(t, db, 1)
}

func rootRepairFixtureCount(t *testing.T, db *gorm.DB, count int) (fixture, fixture, im.Role, im.RolePermission) {
	t.Helper()
	plugin := "com.example.root-catalog"
	p := bootstrapPermission(t, db, plugin, "item")
	rootMember := bootstrapTenant(t, db, plugin, p, false)
	ordinary := bootstrapTenant(t, db, plugin, p, true)
	require.NoError(t, db.Model(&im.User{}).Where("uuid=?", rootMember.member.UserUUID).Update("is_root", true).Error)
	root := im.Role{Scope: "system", Code: "root", Builtin: true}
	require.NoError(t, db.Create(&root).Error)
	stale := im.RolePermission{RoleID: root.ID, RoleUUID: uuid.New(), PermissionID: p.ID, PermissionUUID: p.PermissionUUID, CreatedAt: 42}
	require.NoError(t, db.Create(&stale).Error)
	for i := 1; i < count; i++ {
		extra := im.Permission{Module: "fixture", Resource: fmt.Sprintf("catalog_%04d", i), Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + plugin}
		require.NoError(t, db.Create(&extra).Error)
		require.NoError(t, db.Create(&im.RolePermission{RoleID: root.ID, RoleUUID: stale.RoleUUID, PermissionID: extra.ID, PermissionUUID: extra.PermissionUUID, CreatedAt: 42}).Error)
	}
	past := Micro(time.Now().Add(-24 * time.Hour))
	for _, f := range []fixture{rootMember, ordinary} {
		require.NoError(t, db.Create(&im.AuthorizationTracking{TenantUUID: f.tenant, PluginID: plugin, PermissionCode: "work.item:read", Status: "ready", KnownSince: &past}).Error)
	}
	require.NoError(t, Reconcile(db, past))
	_, segment := rootMember.current(t)
	require.Equal(t, RootBypass, segment.State)
	require.NoError(t, Install(db))
	return rootMember, ordinary, root, stale
}

func TestRootCatalogRepairPreviewApplyHistoryAndIdempotence(t *testing.T) {
	checkRootCatalogRepairHistory(t, bootstrapDB(t))
}

func checkRootCatalogRepairHistory(t *testing.T, db *gorm.DB) {
	t.Helper()
	root, ordinary, role, stale := rootRepairFixture(t, db)
	rootStream, oldRoot := root.current(t)
	ordinaryStream, ordinarySegment := ordinary.current(t)
	preview, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
	require.NoError(t, err)
	require.False(t, preview.Applied)
	require.Len(t, preview.Candidates, 1)
	require.Empty(t, preview.Blocked)
	require.Empty(t, preview.HistoryBlocked)
	require.Len(t, preview.History, 1)
	require.Equal(t, root.tenant, preview.History[0].TenantUUID)
	require.Equal(t, root.member.UUID.String(), preview.History[0].MemberUUID)
	require.Equal(t, rootStream.FactRevision, preview.History[0].BeforeRevision)
	require.Equal(t, rootStream.FactRevision+1, preview.History[0].AfterRevision)
	require.Equal(t, rootStream.KnownSince, preview.History[0].KnownSince)
	var unchanged im.RolePermission
	require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&unchanged).Error)
	require.Equal(t, stale, unchanged)
	before := Micro(time.Now())
	applied, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture-operator"})
	require.NoError(t, err)
	require.True(t, applied.Applied)
	require.Equal(t, 1, applied.Repaired)
	require.Equal(t, "fixture-operator", applied.Operator)
	var repaired im.RolePermission
	require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&repaired).Error)
	require.Equal(t, role.UUID, repaired.RoleUUID)
	require.Equal(t, stale.CreatedAt, repaired.CreatedAt)
	require.Equal(t, stale.PermissionUUID, repaired.PermissionUUID)
	newStream, current := root.current(t)
	require.Equal(t, Allowed, current.State)
	require.Equal(t, "native", current.BaselineKind)
	require.NotNil(t, current.GrantedAt)
	require.False(t, current.GrantedAt.Before(before))
	require.Equal(t, rootStream.UUID, newStream.UUID)
	require.Equal(t, rootStream.KnownSince, newStream.KnownSince)
	require.Equal(t, rootStream.FactRevision+1, newStream.FactRevision)
	var retained im.AuthorizationSegment
	require.NoError(t, db.Where("uuid=?", oldRoot.UUID).Take(&retained).Error)
	require.Equal(t, RootBypass, retained.State)
	require.Equal(t, oldRoot.EffectiveFrom, retained.EffectiveFrom)
	require.Equal(t, current.EffectiveFrom, *retained.EffectiveUntil)
	afterOrdinary, afterSegment := ordinary.current(t)
	require.Equal(t, ordinaryStream, afterOrdinary)
	require.Equal(t, ordinarySegment, afterSegment)
	preview, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
	require.NoError(t, err)
	require.Empty(t, preview.Candidates)
	require.Empty(t, preview.History)
	applied, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture-operator"})
	require.NoError(t, err)
	require.Zero(t, applied.Repaired)
	finalStream, finalSegment := root.current(t)
	require.Equal(t, newStream, finalStream)
	require.Equal(t, current, finalSegment)
}

func TestPostgresRootCatalogRepairPreservesHistoryAndOrdinaryMembers(t *testing.T) {
	checkRootCatalogRepairHistory(t, rootCatalogPostgresDB(t))
}

func rootCatalogPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("POWERX_BASELINE_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POWERX_BASELINE_POSTGRES_DSN")
	}
	old := model.PowerXSchema
	schema := "root_catalog_" + uuid.NewString()[:8]
	model.PowerXSchema = schema
	t.Cleanup(func() { model.PowerXSchema = old })
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &im.Department{}, &im.Group{}, &im.Team{}, &im.Position{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}))
	return db
}

func TestPostgresRootCatalogRepair523DanglingLinks(t *testing.T) {
	db := rootCatalogPostgresDB(t)
	root, ordinary, role, _ := rootRepairFixtureCount(t, db, 523)
	ordinaryStream, ordinarySegment := ordinary.current(t)
	plan, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 523)
	require.Empty(t, plan.Blocked)
	applied, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: plan.Fingerprint, Operator: "fixture-batch"})
	require.NoError(t, err)
	require.Equal(t, 523, applied.Repaired)
	var count int64
	require.NoError(t, db.Model(&im.RolePermission{}).Where("role_uuid=?", role.UUID).Count(&count).Error)
	require.EqualValues(t, 523, count)
	_, segment := root.current(t)
	require.Equal(t, Allowed, segment.State)
	afterStream, afterSegment := ordinary.current(t)
	require.Equal(t, ordinaryStream, afterStream)
	require.Equal(t, ordinarySegment, afterSegment)
}

func TestRootCatalogRepairRejectsStalePlansLiveOwnersAndPermissionConflicts(t *testing.T) {
	for _, name := range []string{"stale_plan", "changed_after_preview", "live_owner", "deleted_owner", "permission_uuid", "confirmation", "nested_source_write", "unknown_root_history"} {
		t.Run(name, func(t *testing.T) {
			db := bootstrapDB(t)
			root, _, role, stale := rootRepairFixture(t, db)
			preview, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
			require.NoError(t, err)
			switch name {
			case "stale_plan":
				_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: "0000000000000000000000000000000000000000000000000000000000000000", Operator: "fixture"})
				require.ErrorIs(t, err, ErrRootRepairPlanConflict)
			case "changed_after_preview", "live_owner", "deleted_owner", "permission_uuid":
				// Simulate offline drift on a raw test connection, not an authorized source write.
				pool := db.Statement.ConnPool.(*transactionPool)
				if name == "changed_after_preview" {
					_, err = pool.ConnPool.ExecContext(context.Background(), "UPDATE "+(&im.RolePermission{}).TableName()+" SET created_at=created_at+1 WHERE role_id=?", role.ID)
				} else if name == "permission_uuid" {
					_, err = pool.ConnPool.ExecContext(context.Background(), "UPDATE "+(&im.RolePermission{}).TableName()+" SET permission_uuid=? WHERE role_id=?", uuid.New(), role.ID)
				} else {
					owner := im.Role{Scope: "tenant", TenantUUID: root.tenant, Code: "fixture-owner"}
					owner.UUID = stale.RoleUUID
					if name == "deleted_owner" {
						owner.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
					}
					_, err = pool.ConnPool.ExecContext(context.Background(), "INSERT INTO "+(&im.Role{}).TableName()+" (uuid,scope,tenant_uuid,code,name,builtin,deleted_at) VALUES (?,?,?,?,?,?,?)", owner.UUID, owner.Scope, owner.TenantUUID, owner.Code, owner.Code, false, owner.DeletedAt)
				}
				require.NoError(t, err)
				if name == "changed_after_preview" {
					_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
					require.ErrorIs(t, err, ErrRootRepairPlanConflict)
					break
				}
				preview, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
				require.NoError(t, err)
				require.NotEmpty(t, preview.Blocked)
				_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
				require.ErrorIs(t, err, ErrRootRepairBlocked)
			case "confirmation":
				_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint})
				require.ErrorContains(t, err, "CONFIRMATION_REQUIRED")
			case "nested_source_write":
				err = db.Transaction(func(tx *gorm.DB) error {
					require.NoError(t, Lock(tx))
					_, e := RepairRootCatalog(context.Background(), tx, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
					return e
				})
				require.ErrorIs(t, err, ErrWriteTransaction)
			case "unknown_root_history":
				require.NoError(t, db.Model(&im.AuthorizationStream{}).Where("member_uuid=?", root.member.UUID.String()).Update("facts_hash", "fixture-untrusted-history").Error)
				_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
				require.ErrorIs(t, err, ErrRootRepairPlanConflict)
				preview, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
				require.NoError(t, err)
				require.NotEmpty(t, preview.HistoryBlocked)
				_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
				require.ErrorContains(t, err, "HISTORY_CONFLICT")
			}
			var row im.RolePermission
			require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&row).Error)
			require.Equal(t, stale.RoleUUID, row.RoleUUID)
			_, current := root.current(t)
			require.Equal(t, RootBypass, current.State)
		})
	}
}

func TestRootCatalogRepairRollsBackLinkAndLedgerTogether(t *testing.T) {
	db := bootstrapDB(t)
	root, _, role, stale := rootRepairFixture(t, db)
	before, segment := root.current(t)
	preview, err := RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{})
	require.NoError(t, err)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture:root-repair-ledger-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == (&im.AuthorizationSegment{}).TableName() || tx.Statement.Table == "iam_authorization_segments" {
			tx.AddError(errors.New("FIXTURE_LEDGER_WRITE_FAILED"))
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove("fixture:root-repair-ledger-fail") })
	_, err = RepairRootCatalog(context.Background(), db, RootCatalogRepairInput{Apply: true, ExpectedFingerprint: preview.Fingerprint, Operator: "fixture"})
	require.ErrorContains(t, err, "FIXTURE_LEDGER_WRITE_FAILED")
	var row im.RolePermission
	require.NoError(t, db.Where("role_id=? AND permission_id=?", role.ID, stale.PermissionID).Take(&row).Error)
	require.Equal(t, stale, row)
	after, current := root.current(t)
	require.Equal(t, before, after)
	require.Equal(t, segment, current)
}
