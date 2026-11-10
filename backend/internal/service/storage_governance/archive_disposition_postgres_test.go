package storagegov

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestArchiveDispositionNativePublishedPauseAndDatabaseFailure(t *testing.T) {
	svc, ctrl, _, _ := monitoringNative(t)
	ctx := controlRoot()
	dir, source := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "partial.jpg"), []byte("owned content"), 0600))
	cfg := archive.Config{Directory: dir, Instance: "owned-disposition", Sources: []archive.Source{{ID: "media", Name: "owned media", Kind: "media", Directory: source}}}
	seed := archive.NewService(context.Background(), cfg, archive.Options{})
	p, e := seed.SavePolicy(ctx, archive.Policy{Name: "partial", SourceID: "media", IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90})
	require.NoError(t, e)
	j, e := seed.Start(ctx, p.ID, uuid.NewString(), "owned", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool { v, e := seed.Job(ctx, j.ID); return e == nil && v.Status == "success" }, 5*time.Second, 20*time.Millisecond)
	seed.Close()
	path := filepath.Join(dir, "jobs", j.ID+".json")
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	var record map[string]any
	require.NoError(t, json.Unmarshal(raw, &record))
	record["status"] = "running"
	record["files"] = nil
	record["phase"] = "capturing"
	raw, e = json.Marshal(record)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	a := archive.NewService(context.Background(), cfg, archive.Options{})
	t.Cleanup(a.Close)
	anchorDir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(anchorDir, 0700))
	anchor, e := recovery.Initialize(anchorDir, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, a.BindRecoveryAnchor(anchor))
	require.NoError(t, svc.BindArchiveControl(a))
	policies := &archivePolicyControl{svc, a}
	adoptNativePolicy(t, policies, ctrl)
	review, e := a.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	// 真正发布另一个暂停版本，旧预览应拒绝；重新核验后才允许处置。
	configuration, e := policies.Configuration(ctx)
	require.NoError(t, e)
	currentPolicy, e := policies.Resolve(ctx, p.ID)
	require.NoError(t, e)
	settings := archive.SettingsOfPolicy(currentPolicy)
	settings.Name += " reviewed"
	_, e = policies.Save(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: configuration.DesiredRevision, ExpectedPolicyRevision: currentPolicy.Revision, PolicyID: p.ID, Action: "save", Settings: &settings})
	require.NoError(t, e)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	staleInput := archive.RetainInterruptedInput{RequestID: uuid.NewString(), ExpectedVersion: review.ReviewVersion, Action: "retain_and_close", Reason: "旧预览不能跨越已发布策略版本变化", Confirm: true}
	_, e = a.RetainInterruptedCapture(ctx, j.ID, staleInput)
	require.Equal(t, 409, dto.StatusCode(e))
	rejected, e := a.InterruptionRequest(ctx, staleInput.RequestID)
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.State)
	review, e = a.ReviewInterruptedCapture(ctx, j.ID, nil)
	require.NoError(t, e)
	// 已发布暂停配置下，处置成功。
	input := archive.RetainInterruptedInput{RequestID: uuid.NewString(), ExpectedVersion: review.ReviewVersion, Action: "retain_and_close", Reason: "原生控制链核验后保留并结束本次采集", Confirm: true}
	out, e := a.RetainInterruptedCapture(ctx, j.ID, input)
	require.NoError(t, e)
	require.Equal(t, "retained", out.State)
	require.True(t, out.CurrentRecordMatches)
	original, e := a.Job(ctx, j.ID)
	require.NoError(t, e)
	require.True(t, original.Protected)
	// 同一数据库中的控制连接独立关闭，不停止数据库，不影响清理测试 schema 的原连接。
	record["id"] = uuid.NewString()
	id := record["id"].(string)
	// 与上方同属受控原任务夹具：新任务拥有独立的无命令执行证明，不能借用旧 ID。
	commandRaw, e := os.ReadFile(filepath.Join(dir, "command-executions", j.ID+".json"))
	require.NoError(t, e)
	var command map[string]any
	require.NoError(t, json.Unmarshal(commandRaw, &command))
	execution := uuid.NewString()
	record["execution_id"] = execution
	command["id"] = id
	command["job_id"] = id
	command["execution_id"] = execution
	commandRaw, e = json.Marshal(command)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "command-executions", id+".json"), commandRaw, 0600))
	record["status"] = "interrupted"
	record["interruption_pending"] = true
	record["interrupted_at"] = time.Now().UTC()
	raw, e = json.Marshal(record)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jobs", id+".json"), raw, 0600))
	next, e := a.ReviewInterruptedCapture(ctx, id, nil)
	require.NoError(t, e)
	offline, e := gorm.Open(postgres.Open(os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := offline.DB()
	require.NoError(t, e)
	require.NoError(t, pool.Close())
	previous := ctrl.repo.DB
	ctrl.repo.DB = offline
	defer func() { ctrl.repo.DB = previous }()
	input.RequestID = uuid.NewString()
	input.ExpectedVersion = next.ReviewVersion
	_, e = a.RetainInterruptedCapture(ctx, id, input)
	require.Error(t, e)
	require.NotEqual(t, 403, dto.StatusCode(e))
	pending, e := a.InterruptionRequest(ctx, input.RequestID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", pending.State)
	current, e := a.Job(ctx, id)
	require.NoError(t, e)
	require.True(t, current.InterruptionPending)
	require.Nil(t, current.CaptureRetention)
	// 当前控制库不可用时禁止人工核对；恢复后新预览不能跨越再次断库。
	_, e = a.ReviewInterruptionRequest(ctx, input.RequestID)
	require.Error(t, e)
	ctrl.repo.DB = previous
	rv, e := a.ReviewInterruptionRequest(ctx, input.RequestID)
	require.NoError(t, e)
	closure := archive.CloseInterruptionRequestInput{RequestID: uuid.NewString(), ExpectedVersion: rv.ReviewVersion, Action: "close_unknown_keep_protection", Reason: "正式控制链恢复后人工保留保护并结案", Confirm: true}
	ctrl.repo.DB = offline
	_, e = a.CloseInterruptionRequest(ctx, input.RequestID, closure)
	require.Error(t, e)
	still, e := a.InterruptionRequest(ctx, input.RequestID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", still.State)
	require.Nil(t, still.Closure)
	ctrl.repo.DB = previous
	closed, e := a.CloseInterruptionRequest(ctx, input.RequestID, closure)
	require.NoError(t, e)
	require.Equal(t, "closed_unknown", closed.State)
	require.True(t, closed.CurrentProtectionMatches)
	current, e = a.Job(ctx, id)
	require.NoError(t, e)
	require.True(t, current.InterruptionPending)
	require.Nil(t, current.CaptureRetention)
	active, e := policies.Resolve(ctx, p.ID)
	require.NoError(t, e)
	active.Enabled = true
	require.Error(t, policies.Guard(ctx, active, "interruption-retain", func([]archive.Policy, *archive.Target) error { return nil }))
}
