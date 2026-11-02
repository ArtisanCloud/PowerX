package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	observer "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func logCleanupFixture(t *testing.T) (*gov.Service, *gorm.DB, string, context.Context, *bool) {
	db := storageNativeDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, name := range []string{"info.log", "info-2020-01-01T00-00-00.000.log", "info-2020-02-01T00-00-00.000.log", "invoice.txt"} {
		p := filepath.Join(root, name)
		require.NoError(t, os.WriteFile(p, []byte("fixture"), 0600))
		require.NoError(t, os.Chtimes(p, old, old))
	}
	legacy := false
	logs := &observer.ApplicationLogs{Sources: map[string]observer.ApplicationLogSource{"app-logs": {Directory: root, ActiveFiles: []string{"info.log"}, RetentionDays: 7, RetentionCount: 10}}, LegacyEnabled: func() bool { return legacy }}
	s, err := gov.NewService(gov.Options{DB: db, Identity: gov.Identity{InstanceID: "test-install", Environment: "dev", HostID: "host", FilesystemID: "disk"}, FilesystemPath: root, Collector: observer.Collector{}, LogLifecycle: logs, Resources: []gov.Resource{{ID: "app-logs", Name: "Application logs", Kind: "application_logs", Scope: "dev", InstanceID: "test-install", Path: root, LogActiveFiles: []string{"info.log"}}}})
	require.NoError(t, err)
	ctx := reqctx.WithIsRoot(context.Background(), true)
	ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 1, IsRoot: true})
	return s, db, root, ctx, &legacy
}
func TestStorageNativeLogCleanupPreviewExecuteReadbackAndIdempotence(t *testing.T) {
	s, _, root, ctx, _ := logCleanupFixture(t)
	preview, err := s.PreviewCleanup(ctx, "app-logs", 7)
	require.NoError(t, err)
	require.True(t, preview.CanExecute)
	require.Len(t, preview.Items, 1)
	require.FileExists(t, filepath.Join(root, preview.Items[0].Name))
	require.Equal(t, 1, preview.Skipped["active_file"])
	require.Equal(t, 1, preview.Skipped["latest_rotation"])
	result, err := s.ExecuteCleanup(ctx, preview.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", result.State)
	require.Equal(t, 1, result.DeletedFiles)
	require.Nil(t, result.ReclaimedBytes)
	require.NoFileExists(t, filepath.Join(root, preview.Items[0].Name))
	for _, name := range []string{"info.log", "invoice.txt", "info-2020-02-01T00-00-00.000.log"} {
		require.FileExists(t, filepath.Join(root, name))
	}
	read, err := s.CleanupRun(ctx, preview.ID)
	require.NoError(t, err)
	require.Equal(t, result, read)
	replayed, err := s.ExecuteCleanup(ctx, preview.ID)
	require.NoError(t, err)
	require.Equal(t, result, replayed)
	rows, err := s.CleanupRuns(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	encoded, _ := json.Marshal(result)
	require.NotContains(t, string(encoded), root)
	require.NotContains(t, string(encoded), "inode")
	claims := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	require.Equal(t, 200, request(t, claims, "GET", "/api/v1/admin/ops/storage/cleanup-runs/"+preview.ID, "", s).Code)
	require.Equal(t, 400, request(t, claims, "POST", "/api/v1/admin/ops/storage/cleanup-previews", `{"resource_id":"app-logs","target_days":7,"path":"/tmp"}`, s).Code)
	require.Equal(t, 400, request(t, claims, "POST", "/api/v1/admin/ops/storage/cleanup-runs", fmt.Sprintf(`{"preview_id":%q}`, preview.ID), s).Code)
	for _, c := range []reqctx.CoreXClaims{{UserID: 1}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 1, IsRoot: true, Platforms: []string{"service_actor"}}} {
		require.Equal(t, 403, request(t, c, "POST", "/api/v1/admin/ops/storage/cleanup-previews", `{"resource_id":"app-logs","target_days":7}`, s).Code)
	}
}
func TestStorageNativeLogCleanupRechecksChangesOwnershipAndPolicy(t *testing.T) {
	s, _, root, ctx, legacy := logCleanupFixture(t)
	p, err := s.PreviewCleanup(ctx, "app-logs", 7)
	require.NoError(t, err)
	*legacy = true
	_, err = s.ExecuteCleanup(ctx, p.ID)
	require.ErrorIs(t, err, gov.ErrCleanupBlocked)
	*legacy = false
	file := filepath.Join(root, p.Items[0].Name)
	require.NoError(t, os.WriteFile(file, []byte("changed since preview"), 0600))
	out, err := s.ExecuteCleanup(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "partial", out.State)
	require.Zero(t, out.DeletedFiles)
	require.FileExists(t, file)
	old := time.Now().Add(-40 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(file, old, old))
	p, err = s.PreviewCleanup(ctx, "app-logs", 7)
	require.NoError(t, err)
	_, err = s.SavePolicy(ctx, 1, gov.PolicyInput{SchemaVersion: 1, ExpectedRevision: 0, State: "draft", Budget: gov.HostBudget{MaxManagedBytes: 90, MinFreeBytes: 20, CriticalFreeBytes: 10}})
	require.NoError(t, err)
	_, err = s.ExecuteCleanup(ctx, p.ID)
	require.ErrorIs(t, err, gov.ErrCleanupConflict)
}
func TestStorageNativeCleanupJournalFailureIsUnknownAndNeverReexecuted(t *testing.T) {
	s, db, root, ctx, _ := logCleanupFixture(t)
	p, err := s.PreviewCleanup(ctx, "app-logs", 7)
	require.NoError(t, err)
	schema := coremodel.PowerXSchema
	// 测试注入：删除后第一次结果写入失败。删除意图已落盘，读回只能是不确定，不能报告 0 项成功或重删。
	require.NoError(t, db.Exec(`CREATE FUNCTION `+schema+`.fail_cleanup_result() RETURNS trigger AS $$ BEGIN IF NEW.version=4 THEN RAISE EXCEPTION 'fixture journal outage'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER cleanup_outage BEFORE UPDATE ON `+schema+`.`+m.TableCleanupRun+` FOR EACH ROW EXECUTE FUNCTION `+schema+`.fail_cleanup_result()`).Error)
	_, err = s.ExecuteCleanup(ctx, p.ID)
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(root, p.Items[0].Name))
	read, err := s.CleanupRun(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", read.State)
	require.Equal(t, "deleting", read.Items[0].Outcome)
	replay, err := s.ExecuteCleanup(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", replay.State)
	for _, item := range replay.Items {
		require.False(t, strings.Contains(item.Name, "/"))
	}
}

func TestStorageNativeLogCleanupExpiryActorAndProductionArchiveGates(t *testing.T) {
	s, db, root, ctx, _ := logCleanupFixture(t)
	p, err := s.PreviewCleanup(ctx, "app-logs", 7)
	require.NoError(t, err)
	other := reqctx.WithIsRoot(context.Background(), true)
	other = reqctx.WithClaims(other, &reqctx.CoreXClaims{UserID: 2, IsRoot: true})
	_, err = s.ExecuteCleanup(other, p.ID)
	require.ErrorIs(t, err, gov.ErrCleanupConflict)
	var row m.CleanupRun
	require.NoError(t, db.Where("run_id = ?", p.ID).First(&row).Error)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(row.Document, &doc))
	doc["record"].(map[string]any)["expires_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, db.Model(&row).Update("document", datatypes.JSON(raw)).Error)
	_, err = s.ExecuteCleanup(ctx, p.ID)
	require.ErrorIs(t, err, gov.ErrCleanupConflict)
	require.FileExists(t, filepath.Join(root, p.Items[0].Name))
	logs := &observer.ApplicationLogs{Sources: map[string]observer.ApplicationLogSource{"prod-logs": {Directory: root, ActiveFiles: []string{"info.log"}}}}
	prod, err := gov.NewService(gov.Options{DB: db, Identity: gov.Identity{InstanceID: "prod-install", Environment: "prod", HostID: "host", FilesystemID: "disk"}, FilesystemPath: root, Collector: observer.Collector{}, LogLifecycle: logs, Resources: []gov.Resource{{ID: "prod-logs", Name: "Production logs", Kind: "application_logs", Scope: "prod", InstanceID: "prod-install", Path: root}}})
	require.NoError(t, err)
	preview, err := prod.PreviewCleanup(ctx, "prod-logs", 7)
	require.NoError(t, err)
	require.False(t, preview.CanExecute)
	require.Equal(t, "archive_verification_required", preview.BlockedReason)
	_, err = prod.ExecuteCleanup(ctx, preview.ID)
	require.ErrorIs(t, err, gov.ErrCleanupBlocked)
}
